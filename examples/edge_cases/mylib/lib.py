"""Imports nested inside every kind of code block the extractor recurses into.

The interesting question for each case isn't *runtime* correctness — Python
has well-defined semantics for delayed imports — but whether the gazelle
plugin's ruff-based AST walk reaches them at all and emits the right deps.

All imports here are stdlib, so the resolved `deps` list on the generated
`py_library` should be empty. If gazelle starts emitting bogus
`@pip//<stdlib>` labels because it missed a stdlib check on a nested
import, this example's idempotency test catches it.
"""

# Module-level baseline.
import json
from pathlib import Path

# Aliased import.
import dataclasses as dc

# Multiple targets on one line (single ImportFrom statement, separate aliases).
import os, errno

# Type-checking-only import. The block doesn't execute at runtime, but the
# ruff parser still sees it and we deliberately treat it as a real import
# so type-checkers (and IDEs) see the symbol.
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from collections.abc import Iterable  # noqa: F401  used only as a type

# Function-body import — the canonical workaround for circular imports.
def lazy_quote(s: str) -> str:
    import urllib.parse

    return urllib.parse.quote(s)


# Class-body import. Uncommon but legal Python; ruff sees it.
class Container:
    import functools

    cached = functools.lru_cache


# Try/except fallback — the dual-import pattern for stdlib feature backports.
# `tomllib` lands in 3.11; `tomli` is the third-party drop-in for older
# versions. We only declare a runtime dep on what's already available, so
# the resolver should NOT emit `@pip//tomli` here (no pyproject.toml
# declares it).
try:
    import tomllib
except ImportError:  # pragma: no cover - we ship 3.12 in CI
    # gazelle:ignore tomli
    import tomli as tomllib

# Module-scope conditional import.
import sys

if sys.version_info >= (3, 11):
    from datetime import UTC
else:
    UTC = None


# Public API.
@dc.dataclass(frozen=True)
class Doc:
    name: str
    body: dict


def parse_doc(name: str, payload: str) -> Doc:
    """Parse a Doc out of a JSON payload."""
    data = json.loads(payload)
    return Doc(name=name, body=data)


def doc_path(home: Path, name: str) -> Path:
    return home / f"{name}.json"


def errno_for_missing() -> int:
    return errno.ENOENT


def is_modern() -> bool:
    return sys.version_info >= (3, 11)


def parse_toml(payload: str) -> dict:
    return tomllib.loads(payload)
