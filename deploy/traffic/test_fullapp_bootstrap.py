"""CI-only prerequisite: two real cmd/server processes, never fake app handlers.

This proves isolated initialization/readiness/routing, NOT model SSE, replay,
account fixtures, billing or the full multi-host feature. No existing config,
credential or database is read. All dynamic operations belong on the CI runner.
"""

import hashlib
import http.client
import json
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import tempfile
import time
import unittest


PG_IMAGE = "postgres:18.1-alpine3.23"
REDIS_IMAGE = "redis:8.4-alpine"
RUNTIME_IMAGE = "alpine:3.23"
TEMPLATE = Path(__file__).with_name("Caddyfile.responses")


class FixtureFailure(RuntimeError):
    pass


def docker_failure_flags(text):
    """Return fixed classifications only; input may contain credentials/paths."""
    text = text.lower()
    signatures = {
        "logging_options": ("log opt", "log-opt", "logging driver", "max-file", "max-size"),
        "cgroup": ("cgroup",),
        "cpu_limit": ("cpu", "cfs"),
        "memory_limit": ("memory", "swap"),
        "process_limit": ("pids", "resource temporarily unavailable"),
        "network_setup": ("network", "iptables", "nftables", "ipam", "veth"),
        "mount_setup": ("mount", "read-only file system"),
        "permission": ("permission denied", "operation not permitted"),
        "runtime_start": ("oci runtime", "runc", "create task", "create shim"),
        "security_policy": ("apparmor", "seccomp"),
        "disk_space": ("no space left",),
        "image_or_binary": ("executable file not found", "no such image", "exec format"),
        "invalid_option": ("unknown flag", "invalid argument", "invalid option"),
    }
    flags = {name: any(signature in text for signature in needles)
             for name, needles in signatures.items()}
    flags["unclassified"] = not any(flags.values())
    return flags


def docker(*args, timeout=30, check=True):
    # Output may contain fixture passwords or config. Never echo Docker argv,
    # stdout/stderr or CalledProcessError to ordinary CI logs/artifacts.
    try:
        result = subprocess.run(["docker", *map(str, args)], capture_output=True,
                                text=True, timeout=timeout, check=False)
    except subprocess.TimeoutExpired:
        raise FixtureFailure("isolated Docker operation exceeded budget") from None
    if check and result.returncode:
        flags = docker_failure_flags(result.stderr)
        raise FixtureFailure("isolated Docker operation failed: exit " + str(result.returncode)
                             + " classifications=" + json.dumps(flags, sort_keys=True))
    return result


def eventually(predicate, seconds, label):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        if predicate():
            return
        time.sleep(0.2)
    raise FixtureFailure("isolated fixture deadline: " + label)


class RealApplicationBootstrapTest(unittest.TestCase):
    def setUp(self):
        self.assertEqual(os.environ.get("CI"), "true", "CI-only; never run on a production host")
        self.binary = Path(os.environ["FULLAPP_BIN"]).resolve(strict=True)
        self.caddy = Path(os.environ["CADDY_BIN"]).resolve(strict=True)
        self.expected_commit = os.environ["FULLAPP_EXPECTED_COMMIT"]
        version = subprocess.run([str(self.binary), "-version"], capture_output=True,
                                 timeout=5, text=True, check=False)
        self.assertEqual(version.returncode, 0)
        self.assertIn("commit: " + self.expected_commit, version.stdout + version.stderr)
        caddy_version = subprocess.run([str(self.caddy), "version"], capture_output=True,
                                       timeout=5, text=True, check=False)
        self.assertEqual(caddy_version.returncode, 0)
        self.assertTrue(caddy_version.stdout.startswith("v2.11.7 "))
        self.temp = tempfile.TemporaryDirectory(prefix="s2-fullapp-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.prefix = "s2-bootstrap-" + secrets.token_hex(5)
        self.network = self.prefix + "-net"
        self.containers = []
        self.report = {"compiled_commit": self.expected_commit,
                       "binary_sha256": hashlib.sha256(self.binary.read_bytes()).hexdigest(),
                       "scope": "full cmd/server initialization; no authenticated model request",
                       "images": [], "phases": [], "containers": []}
        self.addCleanup(self.save_report)
        docker("info", timeout=15)
        # Pull before creating the network; application containers have no
        # external network. These match existing isolated PG/Redis CI versions.
        for image in (PG_IMAGE, REDIS_IMAGE, RUNTIME_IMAGE):
            docker("pull", image, timeout=120)
            image_id = docker("image", "inspect", "--format", "{{.Id}}", image).stdout.strip()
            self.report["images"].append({"tag": image, "id": image_id})
        docker("network", "create", "--internal", "--label", "sub2api.fixture=" + self.prefix,
               self.network)
        self.addCleanup(self.remove_network)
        self.assertEqual(docker("network", "inspect", "--format", "{{.Internal}}",
                                self.network).stdout.strip(), "true")
        self.addCleanup(self.remove_containers)
        (self.root / "bin").mkdir()
        shutil.copyfile(self.binary, self.root / "bin" / "sub2api")
        shutil.copyfile(self.caddy, self.root / "bin" / "caddy")
        for file in (self.root / "bin").iterdir():
            file.chmod(0o755)
        password = secrets.token_hex(24)
        pg_env = self.root / "postgres.env"
        pg_env.write_text("POSTGRES_PASSWORD=" + password + "\nPOSTGRES_DB=bootstrap_fixture\n")
        pg_env.chmod(0o600)
        self.pg = self.run_container("pg", PG_IMAGE, "384m", "1",
                                     ["--env-file", str(pg_env)],
                                     ["postgres", "-c", "shared_buffers=64MB", "-c", "max_connections=40"])
        self.redis = self.run_container("redis", REDIS_IMAGE, "96m", "0.5", [],
                                        ["redis-server", "--save", "", "--appendonly", "no",
                                         "--maxmemory", "64mb", "--maxmemory-policy", "noeviction"])
        eventually(lambda: docker("exec", self.pg, "pg_isready", "-U", "postgres",
                                  "-d", "bootstrap_fixture", timeout=3, check=False).returncode == 0,
                   30, "PostgreSQL ready")
        eventually(lambda: docker("exec", self.redis, "redis-cli", "ping", timeout=3,
                                  check=False).stdout.strip() == "PONG", 15, "Redis ready")
        shared_jwt, shared_totp = secrets.token_hex(32), secrets.token_hex(32)
        self.apps = []
        self.ports = []
        for node in ("a", "b"):
            directory = self.root / node
            directory.mkdir(mode=0o700)
            pricing = directory / "pricing"
            pricing.mkdir()
            (pricing / "model_pricing.json").write_text(json.dumps({
                "gpt-6.1-sol": {"input_cost_per_token": 0, "output_cost_per_token": 0}}))
            cfg = {"timezone": "UTC", "run_mode": "standard",
                   "runtime": {"role": "gateway", "node_id": "bootstrap-" + node},
                   "server": {"host": "0.0.0.0", "port": 8080, "mode": "release",
                              "readiness_timeout_seconds": 1, "shutdown_drain_delay": 4,
                              "graceful_shutdown_timeout": 5},
                   "database": {"host": self.pg, "port": 5432, "user": "postgres",
                                "password": password, "dbname": "bootstrap_fixture", "sslmode": "disable",
                                "max_open_conns": 8, "max_idle_conns": 2},
                   "redis": {"host": self.redis, "port": 6379, "db": 0, "pool_size": 8,
                             "min_idle_conns": 1, "dial_timeout_seconds": 1,
                             "read_timeout_seconds": 1, "write_timeout_seconds": 1},
                   "jwt": {"secret": shared_jwt}, "totp": {"encryption_key": shared_totp},
                   "pricing": {"remote_url": "", "hash_url": "", "data_dir": "/fixture/node/pricing",
                               "fallback_file": "", "update_interval_hours": 24},
                   "log": {"level": "error", "output": {"to_stdout": True, "to_file": False}},
                   "plugins": {"data_dir": "/fixture/node/plugins"}}
            config = directory / "config.yaml"
            config.write_text(json.dumps(cfg))  # JSON is accepted by the YAML loader.
            config.chmod(0o600)
            app = self.run_container("app-" + node, RUNTIME_IMAGE, "512m", "1", [
                "--user", str(os.getuid()) + ":" + str(os.getgid()),
                "--cap-drop=ALL", "--security-opt=no-new-privileges", "--read-only",
                "--tmpfs", "/tmp:rw,noexec,nosuid,size=64m", "--workdir", "/fixture/node",
                "--mount", "type=bind,src=" + str(self.root / "bin") + ",dst=/fixture/bin,readonly",
                "--mount", "type=bind,src=" + str(directory) + ",dst=/fixture/node",
                "--env", "CONFIG_FILE=/fixture/node/config.yaml", "--env", "DATA_DIR=/fixture/node",
                "--env", "SKIP_SETUP=true", "--env", "IP_RISK_DISABLED=true",
                "--env", "GOMAXPROCS=1", "--publish", "127.0.0.1::8080",
                "--entrypoint", "/fixture/bin/sub2api"], [])
            self.apps.append(app)
            port = self.published_port(app, "8080/tcp")
            self.ports.append(port)
            # Real cmd/server initializes migrations and its complete Wire graph.
            eventually(lambda: self.ready(app, port), 120, "full application " + node + " startup")
            identity = json.loads(docker("inspect", "--format",
                                         '{"path":{{json .Path}},"args":{{json .Args}}}', app).stdout)
            self.assertEqual(identity, {"path": "/fixture/bin/sub2api", "args": []})
        pids = [self.state(app)["Pid"] for app in self.apps]
        self.assertTrue(all(pid > 0 for pid in pids))
        self.assertNotEqual(pids[0], pids[1], "must be two full OS application processes")
        self.phase("two-full-applications-ready")
        # The shipping routing/health template is used. A fixture-only response
        # marker reveals which dial target handled an unauthenticated request.
        template = TEMPLATE.read_text()
        needle = "\t\t\theader_up Host "
        self.assertEqual(template.count(needle), 1)
        template = template.replace(needle,
                                    "\t\t\theader_down X-Fixture-Target {upstream_hostport}\n" + needle)
        caddyfile = self.root / "Caddyfile"
        caddyfile.write_text(template)
        self.edge = self.run_container("edge", RUNTIME_IMAGE, "256m", "0.5", [
            "--cap-drop=ALL", "--security-opt=no-new-privileges", "--read-only",
            "--tmpfs", "/tmp:rw,noexec,nosuid,size=32m", "--publish", "127.0.0.1::9080",
            "--mount", "type=bind,src=" + str(self.root / "bin") + ",dst=/fixture/bin,readonly",
            "--mount", "type=bind,src=" + str(caddyfile) + ",dst=/fixture/Caddyfile,readonly",
            "--env", "SUB2API_LB_LISTEN=0.0.0.0:9080",
            "--env", "SUB2API_LB_UPSTREAM_A=" + self.apps[0] + ":8080",
            "--env", "SUB2API_LB_UPSTREAM_B=" + self.apps[1] + ":8080",
            "--env", "SUB2API_LB_ORIGIN_HOST=api.fixture.invalid",
            "--env", "SUB2API_LB_HEALTH_INTERVAL=100ms",
            "--env", "XDG_DATA_HOME=/tmp", "--env", "XDG_CONFIG_HOME=/tmp",
            "--entrypoint", "/fixture/bin/caddy"],
            ["run", "--config", "/fixture/Caddyfile", "--adapter", "caddyfile"])
        self.edge_port = self.published_port(self.edge, "9080/tcp")

    def run_container(self, suffix, image, memory, cpus, options, command):
        name = self.prefix + "-" + suffix
        # Register before create/run so failure cleanup also catches a container
        # created successfully before a start error or deadline.
        self.containers.append(name)
        docker("run", "--detach", "--name", name, "--network", self.network,
               "--label", "sub2api.fixture=" + self.prefix, "--memory", memory,
               "--memory-swap", memory, "--cpus", cpus, "--pids-limit", "128",
               "--log-driver", "local", "--log-opt", "max-size=1m", "--log-opt", "max-file=1",
               *options, image, *command)
        actual = json.loads(docker("inspect", "--format",
                                   '{"memory":{{.HostConfig.Memory}},"swap":{{.HostConfig.MemorySwap}},'
                                   '"network":{{json .HostConfig.NetworkMode}}}', name).stdout)
        self.assertEqual(actual["memory"], int(memory[:-1]) * 1024 * 1024)
        self.assertEqual(actual["swap"], actual["memory"])
        self.assertEqual(actual["network"], self.network)
        return name

    def published_port(self, name, port):
        bindings = json.loads(docker("inspect", "--format", "{{json .NetworkSettings.Ports}}",
                                     name).stdout)[port]
        self.assertEqual(len(bindings), 1)
        self.assertEqual(bindings[0]["HostIp"], "127.0.0.1")
        return int(bindings[0]["HostPort"])

    def state(self, name):
        state = json.loads(docker("inspect", "--format", "{{json .State}}", name).stdout)
        return {key: state[key] for key in ("Running", "Paused", "OOMKilled", "ExitCode", "Pid")}

    def request(self, port, path="/readyz", method="GET"):
        connection = http.client.HTTPConnection("127.0.0.1", port, timeout=2)
        try:
            connection.request(method, path, body="{}" if method == "POST" else None,
                               headers={"Host": "api.fixture.invalid", "Content-Type": "application/json"})
            response = connection.getresponse()
            response.read(4096)
            return response.status, response.getheader("X-Fixture-Target")
        except (OSError, http.client.HTTPException):
            return None, None
        finally:
            connection.close()

    def ready(self, app, port):
        state = self.state(app)
        if not state["Running"]:
            raise FixtureFailure("full application exited before ready; OOM=" + str(state["OOMKilled"]))
        return self.request(port)[0] == 200

    def phase(self, name):
        self.report["phases"].append(name)
        print("isolated full-application phase: " + name, flush=True)

    def remove_containers(self):
        failures = []
        for name in reversed(self.containers):
            # Only collect numeric/fixed state from this fixture, never env,
            # mounts, config, SQL, application logs or request contents.
            try:
                inspected = docker("inspect", "--format", "{{json .State}}", name, timeout=5, check=False)
                if inspected.returncode != 0:
                    continue  # run may have failed before create.
                state = json.loads(inspected.stdout)
                self.report["containers"].append({"role": name.removeprefix(self.prefix + "-"),
                                                   "OOMKilled": state["OOMKilled"],
                                                   "ExitCode": state["ExitCode"],
                                                   "start_error_flags": docker_failure_flags(state.get("Error", ""))})
                if "-app-" in name:
                    # Classify ONLY this synthetic application's bounded log.
                    # Export fixed booleans, never raw errors/config/SQL/URLs.
                    logs = docker("logs", "--tail", "60", name, timeout=5, check=False)
                    text = logs.stdout + logs.stderr
                    self.report["containers"][-1]["diagnostic_flags"] = {
                        code: marker in text for code, marker in (
                            ("config_load_failed", "Failed to load config"),
                            ("application_init_failed", "Failed to initialize application"),
                            ("migration_failed", "migration failed"),
                            ("read_only_failure", "read-only file system"),
                            ("server_started", "Server started"),
                            ("server_exited", "Server exited"))}
                if state["Paused"]:
                    docker("unpause", name, timeout=5, check=False)
            except (FixtureFailure, json.JSONDecodeError):
                failures.append("container state unavailable during cleanup")
            try:
                if docker("rm", "--force", "--volumes", name, timeout=20, check=False).returncode:
                    failures.append("container cleanup failed")
            except FixtureFailure:
                failures.append("container cleanup exceeded budget")
        if failures:
            raise FixtureFailure("; ".join(failures))

    def remove_network(self):
        result = docker("network", "rm", self.network, timeout=15, check=False)
        self.assertEqual(result.returncode, 0, "isolated network must be removed")

    def save_report(self):
        # Called after container cleanup, before the restricted temp files vanish.
        report = os.environ.get("FULLAPP_REPORT")
        if report:
            Path(report).write_text(json.dumps(self.report, indent=2) + "\n")

    def test_full_application_bootstrap_readiness_and_signal_drain(self):
        expected = {app + ":8080" for app in self.apps}
        seen = set()

        def both_routed():
            code, target = self.request(self.edge_port, "/v1/responses", "POST")
            if code == 401 and target in expected:
                seen.add(target)
            return seen == expected

        eventually(both_routed, 10, "real Caddy must reach both real app auth routes")
        self.phase("two-real-app-auth-routes-through-caddy-401")
        # No model or API Key is used: 401 is only proof of real routing/auth
        # initialization, never proof of a successful model/SSE request.
        docker("pause", self.redis)
        try:
            for port in self.ports:
                eventually(lambda: self.request(port)[0] == 503, 10, "Redis dependency withdraws readiness")
            eventually(lambda: self.request(self.edge_port, "/v1/responses", "POST")[0] == 503,
                       10, "Caddy withdraws both unready full applications")
            self.phase("real-redis-unavailable-readiness-and-edge-503")
        finally:
            docker("unpause", self.redis)
        for app, port in zip(self.apps, self.ports):
            eventually(lambda app=app, port=port: self.ready(app, port), 15, "full app readiness recovers")
        seen.clear()
        eventually(both_routed, 15, "Caddy restores both recovered full applications")
        self.phase("real-redis-and-both-app-routes-recovered")
        docker("kill", "--signal=TERM", self.apps[0])
        # Real main.go calls BeginDrain, delay, Shutdown, Lifecycle.Wait, Cleanup.
        target = self.apps[1] + ":8080"
        consecutive = 0

        def peer_only():
            nonlocal consecutive
            code, actual = self.request(self.edge_port, "/v1/responses", "POST")
            consecutive = consecutive + 1 if code == 401 and actual == target else 0
            return consecutive >= 4

        eventually(peer_only, 12, "real signal drain withdraws only the stopped application")
        result = docker("wait", self.apps[0], timeout=20)
        self.assertEqual(result.stdout.strip(), "0", "real main must shut down cleanly")
        self.assertFalse(self.state(self.apps[0])["OOMKilled"])
        self.assertTrue(self.state(self.apps[1])["Running"])
        self.phase("real-sigterm-drain-clean-exit-peer-still-serving")


class DockerFailureClassificationTest(unittest.TestCase):
    def test_fixed_diagnostics_do_not_echo_sensitive_input(self):
        sensitive = "token=fixture-secret /private/fixture-path 192.0.2.7"
        for message, expected in (
                ("invalid log opt max-file " + sensitive, "logging_options"),
                ("OCI runtime failed to create task: pids limit " + sensitive, "process_limit"),
                ("network iptables permission denied " + sensitive, "network_setup"),
                ("unexpected failure " + sensitive, "unclassified")):
            with self.subTest(category=expected):
                flags = docker_failure_flags(message)
                self.assertTrue(flags[expected])
                encoded = json.dumps(flags)
                for value in (sensitive, "fixture-secret", "/private/fixture-path", "192.0.2.7"):
                    self.assertNotIn(value, encoded)
                self.assertTrue(all(isinstance(value, bool) for value in flags.values()))


if __name__ == "__main__":
    unittest.main(verbosity=2)
