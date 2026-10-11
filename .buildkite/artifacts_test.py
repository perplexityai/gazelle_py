import importlib.util
import json
import os
import pathlib
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("artifacts", pathlib.Path(__file__).with_name("artifacts.py"))
artifacts = importlib.util.module_from_spec(spec)
spec.loader.exec_module(artifacts)


class ArtifactTests(unittest.TestCase):
    def setUp(self):
        self.work = tempfile.TemporaryDirectory()
        self.addCleanup(self.work.cleanup)
        self.directory = pathlib.Path(self.work.name)
        environment = dict(BUILDKITE_COMMIT="a" * 40, BUILDKITE_BUILD_ID="build-1",
                           BUILDKITE_PIPELINE_SLUG="pipeline", USE_BAZEL_VERSION="9.0.0")
        self.environment = patch.dict(os.environ, environment)
        self.environment.start()
        self.addCleanup(self.environment.stop)
        for name in artifacts.FILES:
            (self.directory / name).write_bytes(b"binary")
        artifacts.process("pack", self.directory, "darwin-arm64")

    def verify(self):
        artifacts.process("verify", self.directory, "darwin-arm64")

    def test_valid(self):
        self.verify()
        for name in artifacts.FILES:
            self.assertTrue(os.access(self.directory / name, os.X_OK))

    def test_identity_mismatch(self):
        for key in ("BUILDKITE_COMMIT", "BUILDKITE_BUILD_ID", "BUILDKITE_PIPELINE_SLUG", "USE_BAZEL_VERSION"):
            with self.subTest(key=key), patch.dict(os.environ, {key: "different"}):
                with self.assertRaisesRegex(ValueError, "identity"):
                    self.verify()

    def test_wrong_platform(self):
        with self.assertRaisesRegex(ValueError, "identity"):
            artifacts.process("verify", self.directory, "linux-amd64")

    def test_tampered_binary(self):
        (self.directory / artifacts.FILES[0]).write_bytes(b"tampered")
        with self.assertRaisesRegex(ValueError, "checksum"):
            self.verify()

    def test_missing_binary(self):
        (self.directory / artifacts.FILES[0]).unlink()
        with self.assertRaisesRegex(ValueError, "checksum"):
            self.verify()

    def test_symlink(self):
        binary = self.directory / artifacts.FILES[0]
        binary.rename(self.directory / "outside")
        binary.symlink_to(self.directory / "outside")
        with self.assertRaisesRegex(ValueError, "checksum"):
            self.verify()

    def test_extra_manifest_file(self):
        path = self.directory / "manifest.json"
        manifest = json.loads(path.read_text())
        manifest["files"]["unexpected"] = "hash"
        path.write_text(json.dumps(manifest))
        with self.assertRaisesRegex(ValueError, "Unexpected"):
            self.verify()


if __name__ == "__main__":
    unittest.main()
