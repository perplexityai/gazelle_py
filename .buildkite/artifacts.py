#!/usr/bin/env python3
"""Bind smoke binaries to their producing build and verify before execution."""
import hashlib
import json
import os
import pathlib
import sys

FILES = ("gazelle", "unit-test")


def identity(platform):
    return {key: os.environ[env] for key, env in (
        ("commit", "BUILDKITE_COMMIT"), ("build", "BUILDKITE_BUILD_ID"),
        ("pipeline", "BUILDKITE_PIPELINE_SLUG"), ("bazel", "USE_BAZEL_VERSION"),
    )} | {"platform": platform}


def digest(path):
    sha = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1024 * 1024), b""):
            sha.update(block)
    return sha.hexdigest()


def process(mode, directory, platform):
    directory = pathlib.Path(directory)
    expected = identity(platform)
    if mode == "pack":
        expected["files"] = {name: digest(directory / name) for name in FILES}
        (directory / "manifest.json").write_text(json.dumps(expected, sort_keys=True) + "\n")
    elif mode == "verify":
        manifest = json.loads((directory / "manifest.json").read_text())
        if any(manifest.get(key) != value for key, value in expected.items()):
            raise ValueError("Artifact identity does not match this build")
        if set(manifest.get("files", {})) != set(FILES):
            raise ValueError("Unexpected artifact files")
        for name in FILES:
            path = directory / name
            if path.is_symlink() or not path.is_file() or digest(path) != manifest["files"][name]:
                raise ValueError("Artifact checksum mismatch: " + name)
            path.chmod(0o755)
    else:
        raise ValueError("Unknown artifact operation")


if __name__ == "__main__":
    process(*sys.argv[1:])
