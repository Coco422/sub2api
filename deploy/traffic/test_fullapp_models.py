"""CI only: authenticated Responses SSE through two UNMODIFIED cmd/server apps.

The model is synthetic and internal; SQL/account/key/dispatch/billing/concurrency
are the application's real paths. This first slice establishes legal fixtures
and normal SSE settlement, before drain/EOF/partial/cancel fault scenarios.
"""

import http.client
import json
import os
from pathlib import Path
import secrets
import shutil
import unittest

import test_fullapp_bootstrap as bootstrap


class FullApplicationModelSSETest(bootstrap.RealApplicationBootstrapTest):
    def fixture_pricing(self):
        # Nonzero price makes an empty billing path observable. No money/model
        # provider exists outside this exclusive database/network fixture.
        return {"gpt-6.1-sol": {"input_cost_per_token": 0.000001,
                                "output_cost_per_token": 0.000002}}

    def configure_application(self, cfg):
        # Only the already internal Docker fixture can use plaintext upstreams;
        # production defaults/config are unchanged, no skip-certificate option.
        cfg["security"] = {"url_allowlist": {"enabled": False,
                                              "allow_private_hosts": True,
                                              "allow_insecure_http": True}}

    def setUp(self):
        super().setUp()
        tool = Path(os.environ["FULLAPP_FIXTURE_BIN"]).resolve(strict=True)
        destination = self.root / "bin" / "fullapp-fixture"
        shutil.copyfile(tool, destination)
        destination.chmod(0o755)
        self.model = self.run_container("model", bootstrap.RUNTIME_IMAGE, "256m", "0.5", [
            "--user", str(os.getuid()) + ":" + str(os.getgid()),
            "--cap-drop=ALL", "--security-opt=no-new-privileges", "--read-only",
            "--mount", "type=bind,src=" + str(self.root / "bin") + ",dst=/fixture/bin,readonly",
            "--env", "CI_FULLAPP_FIXTURE=1", "--env", "GOMAXPROCS=1",
            "--entrypoint", "/fixture/bin/fullapp-fixture"], ["serve"])
        self.model_endpoint = self.internal_endpoint(self.model, 9090)
        bootstrap.eventually(lambda: self.model_stats() is not None, 5, "synthetic model ready")
        cfg = json.loads((self.root / "a" / "config.yaml").read_text())
        self.key = "sk-fixture-" + secrets.token_hex(16)
        self.fixture_input = {"database": {name: cfg["database"][name]
                                           for name in ("host", "port", "user", "password", "dbname", "sslmode")},
                              "redis": {name: cfg["redis"][name] for name in ("host", "port", "db")},
                              "api_key": self.key, "origin": "http://" + self.model + ":9090"}
        ids = self.fixture_command("seed")
        for field in ("user_id", "api_key_id", "account_id"):
            self.assertGreater(ids[field], 0, "real repository fixture ID required")
            self.fixture_input[field] = ids[field]
        self.phase("legal-shared-user-group-key-account-and-snapshot-seeded")

    def fixture_command(self, operation):
        # stdin only: no key/config/DSN in Docker argv or report. The helper
        # rejects endpoints that aren't this fixture's generated PG/Redis names.
        result = bootstrap.docker("exec", "--interactive", "--env", "CI_FULLAPP_FIXTURE=1",
                                  self.apps[0], "/fixture/bin/fullapp-fixture", operation,
                                  input_text=json.dumps(self.fixture_input), timeout=15, check=False)
        if result.returncode:
            stages = ("input", "create-user", "create-group", "create-key", "create-account-group",
                      "hydrate-account", "publish-snapshot", "observe-usage-billing-slots")
            stage = next((value for value in stages if "stage=" + value in result.stderr), "unknown")
            raise bootstrap.FixtureFailure("isolated seed/status failed stage=" + stage)
        try:
            value = json.loads(result.stdout)
        except (ValueError, TypeError):
            raise bootstrap.FixtureFailure("isolated seed/status produced no valid fixed result") from None
        self.assertIsInstance(value, dict)
        return value

    def model_stats(self):
        connection = http.client.HTTPConnection(*self.model_endpoint, timeout=2)
        try:
            connection.request("GET", "/fixture/stats")
            response = connection.getresponse()
            if response.status != 200:
                return None
            result = json.loads(response.read(4096))
            return {name: result[name] for name in ("accepted", "completed", "active")}
        except (OSError, http.client.HTTPException, ValueError, KeyError):
            return None
        finally:
            connection.close()

    def test_two_real_applications_complete_sse_once_and_settle_shared_state(self):
        expected = {app + ":8080" for app in self.apps}
        seen = set()
        # Exactly four generated requests, never retry a POST to warm the edge.
        for number in range(1, 5):
            connection = http.client.HTTPConnection(*self.edge_port, timeout=8)
            self.addCleanup(connection.close)
            body = {"model": "gpt-6.1-sol", "input": "synthetic fixture", "stream": True}
            connection.request("POST", "/v1/responses", body=json.dumps(body), headers={
                "Host": "api.fixture.invalid", "Content-Type": "application/json",
                "Authorization": "Bearer " + self.key,
                "X-Client-Request-ID": "model-fixture-" + str(number)})
            response = connection.getresponse()
            self.addCleanup(response.close)
            self.assertEqual(response.status, 200, "real authenticated gateway must return SSE")
            self.assertIn("text/event-stream", response.getheader("Content-Type", ""))
            target = response.getheader("X-Fixture-Target")
            self.assertIn(target, expected, "real Caddy dial target must be one full application")
            seen.add(target)
            payload = response.read(65536)
            self.assertIn(b'"response.completed"', payload)
            self.assertIn(b"OK", payload)
            response.close()
            connection.close()

            def settled():
                stats = self.model_stats()
                if stats != {"accepted": number, "completed": number, "active": 0}:
                    return False
                status = self.fixture_command("status")
                # Exact row counts and nonzero balance debit prevent a mere
                # HTTP200 or bypassed billing from passing this prerequisite.
                return (status["usage_rows"] == number and status["dedup_rows"] == number
                        and status["input_tokens"] == 7 * number and status["output_tokens"] == 2 * number
                        and status["balance"] < 10 and status["balance"] > 9
                        and status["user_slots"] == status["account_slots"] == status["api_key_slots"] == 0)

            bootstrap.eventually(settled, 12, "exact model generation/usage/dedup/debit/slots settled")
        self.assertEqual(seen, expected, "both full applications must process authenticated model requests")
        self.report["model_verification"] = {"generated": 4, "completed": 4,
                                              "usage_rows": 4, "dedup_rows": 4,
                                              "user_account_key_slots": 0,
                                              "scope": "synthetic internal model only; normal SSE prerequisite"}
        self.phase("both-full-app-model-sse-and-unique-settlement-verified")


if __name__ == "__main__":
    # The original bootstrap suite still runs as its existing CI step. This
    # targeted entry adds only the new model case, not inherited duplicates.
    suite = unittest.TestSuite([FullApplicationModelSSETest(
        "test_two_real_applications_complete_sse_once_and_settle_shared_state")])
    result = unittest.TextTestRunner(verbosity=2).run(suite)
    raise SystemExit(0 if result.wasSuccessful() else 1)
