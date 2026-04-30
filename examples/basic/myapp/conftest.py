"""Shared fixtures for the myapp tests.

Exercises the conftest extraction path: gazelle should emit a dedicated
`py_library` named `:conftest` with `testonly = True`, NOT bundle this file
into `:myapp`'s srcs. Tests pick the fixture up via Bazel's transitive deps,
mirroring pytest's automatic conftest discovery.
"""

import json
import tempfile
from pathlib import Path


def write_config_file(name: str = "demo", home: str = "/tmp") -> Path:
    f = tempfile.NamedTemporaryFile("w", suffix=".json", delete=False)
    json.dump({"name": name, "home": home}, f)
    f.close()
    return Path(f.name)
