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
                                  getattr(self, "fixture_executor", self.apps[0]),
                                  "/fixture/bin/fullapp-fixture", operation,
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


class FullApplicationInflightDrainTest(FullApplicationModelSSETest):
    def model_post(self, marker, hold=False):
        connection = http.client.HTTPConnection(*self.edge_port, timeout=12)
        self.addCleanup(connection.close)
        body = {"model": "gpt-6.1-sol", "input": "fixture-hold-until-release" if hold else "synthetic fixture",
                "stream": True}
        connection.request("POST", "/v1/responses", body=json.dumps(body), headers={
            "Host": "api.fixture.invalid", "Content-Type": "application/json",
            "Authorization": "Bearer " + self.key, "X-Client-Request-ID": marker})
        response = connection.getresponse()
        self.addCleanup(response.close)
        self.assertEqual(response.status, 200)
        self.assertIn("text/event-stream", response.getheader("Content-Type", ""))
        return connection, response

    def release_model(self):
        connection = http.client.HTTPConnection(*self.model_endpoint, timeout=2)
        try:
            connection.request("POST", "/fixture/release", body="")
            response = connection.getresponse()
            response.read(1024)
            self.assertEqual(response.status, 204)
        finally:
            connection.close()

    def test_inflight_sse_finishes_on_draining_owner_and_new_model_reaches_peer_once(self):
        old_connection, old_response = self.model_post("drain-old-stream", hold=True)
        owner_target = old_response.getheader("X-Fixture-Target")
        self.assertIn(owner_target, {app + ":8080" for app in self.apps})
        owner = next(index for index, app in enumerate(self.apps) if app + ":8080" == owner_target)
        peer = 1 - owner
        # Read an actual delta before signaling; a completed/buffered response
        # cannot satisfy the in-flight checkpoint.
        for _ in range(12):
            line = old_response.readline(4096)
            self.assertTrue(line, "held stream must stay open until release")
            if b"response.output_text.delta" in line:
                break
        else:
            self.fail("held model did not emit its first text delta")
        self.assertEqual(self.model_stats(), {"accepted": 1, "completed": 0, "active": 1})
        status = self.fixture_command("status")
        self.assertEqual(status["usage_rows"], 0)
        self.assertEqual(status["dedup_rows"], 0)
        self.assertEqual((status["user_slots"], status["account_slots"], status["api_key_slots"]), (1, 1, 1))
        self.phase("model-sse-held-inflight-with-real-shared-slots")

        bootstrap.docker("kill", "--signal=TERM", self.apps[owner])
        bootstrap.eventually(lambda: self.request(self.ports[owner])[0] == 503, 2,
                             "real owner withdraws readiness before listener closes")
        consecutive = 0

        def peer_admitted():
            nonlocal consecutive
            code, target = self.request(self.edge_port, "/v1/responses", "POST")
            consecutive = consecutive + 1 if code == 401 and target == self.apps[peer] + ":8080" else 0
            return consecutive >= 4

        # These are unauthenticated health-routing observations, not model
        # retries. Only two authenticated model POSTs exist in this scenario.
        bootstrap.eventually(peer_admitted, 3, "Caddy observes drain and routes new requests to peer")
        self.assertEqual(self.model_stats(), {"accepted": 1, "completed": 0, "active": 1})
        self.assertTrue(self.state(self.apps[owner])["Running"], "old SSE owner remains alive while draining")
        new_connection, new_response = self.model_post("drain-new-peer")
        self.assertEqual(new_response.getheader("X-Fixture-Target"), self.apps[peer] + ":8080")
        self.assertIn(b'"response.completed"', new_response.read(65536))
        new_response.close()
        new_connection.close()
        self.assertEqual(self.model_stats(), {"accepted": 2, "completed": 1, "active": 1})
        self.phase("new-authenticated-model-completed-on-peer-old-stream-still-held")
        self.release_model()
        self.assertIn(b'"response.completed"', old_response.read(65536))
        old_response.close()
        old_connection.close()
        wait = bootstrap.docker("wait", self.apps[owner], timeout=20)
        self.assertEqual(wait.stdout.strip(), "0")
        self.assertFalse(self.state(self.apps[owner])["OOMKilled"])
        self.assertTrue(self.state(self.apps[peer])["Running"])

        # The seed/status binary executes inside the peer if app-a was drained.
        self.fixture_executor = self.apps[peer]

        def settled():
            if self.model_stats() != {"accepted": 2, "completed": 2, "active": 0}:
                return False
            status = self.fixture_command("status")
            return (status["usage_rows"] == 2 and status["dedup_rows"] == 2
                    and status["input_tokens"] == 14 and status["output_tokens"] == 4
                    and 9 < status["balance"] < 10
                    and status["user_slots"] == status["account_slots"] == status["api_key_slots"] == 0)

        bootstrap.eventually(settled, 12, "both requests settle once and draining-owner slots release")
        self.report["inflight_drain_verification"] = {"authenticated_posts": 2, "generated": 2,
                                                       "completed": 2, "usage_rows": 2, "dedup_rows": 2,
                                                       "slots": 0, "old_stream_migrated_or_replayed": False,
                                                       "owner_exit_code": 0, "peer_serving": True}
        self.phase("old-model-completed-on-original-owner-once-and-drain-settlement-clean")


class FullApplicationPartialEOFTest(FullApplicationModelSSETest):
    def send_model(self, marker, request_id):
        connection = http.client.HTTPConnection(*self.edge_port, timeout=8)
        self.addCleanup(connection.close)
        connection.request("POST", "/v1/responses", body=json.dumps({
            "model": "gpt-6.1-sol", "input": marker, "stream": True}), headers={
                "Host": "api.fixture.invalid", "Content-Type": "application/json",
                "Authorization": "Bearer " + self.key, "X-Client-Request-ID": request_id,
                "Idempotency-Key": request_id})
        response = connection.getresponse()
        self.addCleanup(response.close)
        self.assertEqual(response.status, 200)
        self.assertIn("text/event-stream", response.getheader("Content-Type", ""))
        target = response.getheader("X-Fixture-Target")
        self.assertIn(target, {app + ":8080" for app in self.apps})
        try:
            body = response.read(65536)
        except http.client.IncompleteRead as error:
            body = error.partial
        response.close()
        connection.close()
        return target, body

    def test_partial_model_sse_eof_is_not_replayed_or_billed_and_peer_can_continue(self):
        failed_target, partial = self.send_model("fixture-partial-sse-eof", "partial-eof-fixture")
        self.assertIn(b'"response.output_text.delta"', partial)
        self.assertIn(b"OK", partial)
        self.assertNotIn(b'"response.completed"', partial)
        self.assertNotIn(b"data: [DONE]", partial)

        def failed_settled():
            if self.model_stats() != {"accepted": 1, "completed": 0, "active": 0}:
                return False
            status = self.fixture_command("status")
            return (status["usage_rows"] == status["dedup_rows"] == 0
                    and status["input_tokens"] == status["output_tokens"] == 0
                    and status["balance"] == 10
                    and status["user_slots"] == status["account_slots"] == status["api_key_slots"] == 0)

        bootstrap.eventually(failed_settled, 12, "partial EOF has one generation, no usage/debit, slots released")
        self.phase("partial-model-eof-one-generation-no-terminal-no-billing-slots-clean")
        # A separate explicit client request, not replay/recovery of the old
        # request; this is exactly the second and last model POST in the case.
        control_target, complete = self.send_model("synthetic control", "partial-eof-control")
        self.assertNotEqual(control_target, failed_target, "healthy round-robin peer must serve the new request")
        self.assertIn(b'"response.completed"', complete)

        def control_settled():
            if self.model_stats() != {"accepted": 2, "completed": 1, "active": 0}:
                return False
            status = self.fixture_command("status")
            return (status["usage_rows"] == status["dedup_rows"] == 1
                    and status["input_tokens"] == 7 and status["output_tokens"] == 2
                    and 9 < status["balance"] < 10
                    and status["user_slots"] == status["account_slots"] == status["api_key_slots"] == 0)

        bootstrap.eventually(control_settled, 12, "peer control settles once, failed partial remains unbilled")
        self.report["partial_eof_verification"] = {"authenticated_posts": 2, "accepted": 2,
                                                    "completed": 1, "failed_request_replayed": False,
                                                    "failed_request_billed": False, "usage_rows": 1,
                                                    "dedup_rows": 1, "slots": 0, "peer_control_completed": True}
        self.phase("new-peer-model-after-partial-eof-settled-once-no-hidden-replay")


if __name__ == "__main__":
    # The original bootstrap suite still runs as its existing CI step. This
    # targeted entry adds only the new model case, not inherited duplicates.
    suite = unittest.TestSuite([
        FullApplicationModelSSETest("test_two_real_applications_complete_sse_once_and_settle_shared_state"),
        FullApplicationInflightDrainTest("test_inflight_sse_finishes_on_draining_owner_and_new_model_reaches_peer_once"),
        FullApplicationPartialEOFTest("test_partial_model_sse_eof_is_not_replayed_or_billed_and_peer_can_continue")])
    result = unittest.TextTestRunner(verbosity=2).run(suite)
    raise SystemExit(0 if result.wasSuccessful() else 1)
