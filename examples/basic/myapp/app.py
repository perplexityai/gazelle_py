"""Tiny stdlib-only app exercising the gazelle_py plugin's basic resolution.

Imports cover stdlib top-levels (`json`, `pathlib`, `dataclasses`) plus a
dotted submodule (`os.path`) — the resolver should drop all of them since
they're stdlib, leaving the generated `py_library` with no `deps`.
"""

import json
import os.path
from dataclasses import dataclass
from pathlib import Path


@dataclass
class Config:
    name: str
    home: Path


def load_config(path: str) -> Config:
    with open(path, "r") as f:
        data = json.load(f)
    return Config(name=data["name"], home=Path(os.path.expanduser(data["home"])))
