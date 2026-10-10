"""Release status and moved-tag regression checks."""

import copy
import importlib.util
from pathlib import Path
import subprocess
import unittest

spec = importlib.util.spec_from_file_location("gate", Path(__file__).with_name("buildkite-release-gate.py"))
gate = importlib.util.module_from_spec(spec)
spec.loader.exec_module(gate)


class ReleaseGateTest(unittest.TestCase):
    def setUp(self):
        self.status = {
            "context": "release-ready/gazelle-py", "state": "success",
            "creator": {"login": "buildkite[bot]"},
            "target_url": "https://buildkite.com/perplexity/gazelle-py/builds/24",
        }

    def test_main_success(self):
        self.assertTrue(gate.check_status([self.status], "gazelle-py"))

    def test_pr_success_does_not_qualify(self):
        self.status["context"] = "buildkite/gazelle-py"
        self.assertFalse(gate.check_status([self.status], "gazelle-py"))

    def test_missing_status_waits(self):
        self.assertFalse(gate.check_status([], "gazelle-py"))

    def test_pending_rebuild_overrides_old_success(self):
        pending = copy.deepcopy(self.status)
        pending["state"] = "pending"
        self.assertFalse(gate.check_status([pending, self.status], "gazelle-py"))

    def test_failed_rebuild_overrides_old_success(self):
        failed = copy.deepcopy(self.status)
        failed["state"] = "failure"
        with self.assertRaises(ValueError):
            gate.check_status([failed, self.status], "gazelle-py")

    def test_forged_publisher_rejected(self):
        self.status["creator"]["login"] = "someone-else"
        with self.assertRaises(ValueError):
            gate.check_status([self.status], "gazelle-py")

    def test_other_pipeline_rejected(self):
        self.status["target_url"] = "https://buildkite.com/perplexity/gazelle-css/builds/24"
        with self.assertRaises(ValueError):
            gate.check_status([self.status], "gazelle-py")

    def test_tag_checkout_unchanged(self):
        commit = subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip()
        subprocess.run(["python3", str(Path(__file__).with_name("buildkite-release-gate.py")),
                        "--expected-commit", commit, "--disk_cache=unused", "--repository_cache=unused"], check=True)

    def test_moved_tag_rejected(self):
        result = subprocess.run(["python3", str(Path(__file__).with_name("buildkite-release-gate.py")),
                                 "--expected-commit", "0" * 40], capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Release tag moved", result.stderr)


if __name__ == "__main__":
    unittest.main()
