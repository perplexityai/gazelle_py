"""Imports nested in every block kind the ruff visitor recurses into.

If gazelle ever stops walking into a particular block we'll either miss a
real dep (false negative) or emit a bogus `@pip//<stdlib>` for a stdlib
module (false positive). Both regressions show up here as a goldens diff.
"""

import json
from pathlib import Path

import dataclasses as dc

import os, errno

from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from collections.abc import Iterable  # noqa: F401  used only as a type


def lazy_quote(s: str) -> str:
    import urllib.parse

    return urllib.parse.quote(s)


class Container:
    import functools

    cached = functools.lru_cache


# tomllib lands in 3.11; tomli is the third-party drop-in. We don't ship a
# pyproject declaring tomli, so the `# gazelle:ignore` keeps the resolver
# from emitting `@pip//tomli`.
try:
    import tomllib
except ImportError:  # pragma: no cover
    # gazelle:ignore tomli
    import tomli as tomllib

import sys

if sys.version_info >= (3, 11):
    from datetime import UTC
else:
    UTC = None


@dc.dataclass(frozen=True)
class Doc:
    name: str
    body: dict


def parse_doc(name: str, payload: str) -> Doc:
    data = json.loads(payload)
    return Doc(name=name, body=data)


def doc_path(home: Path, name: str) -> Path:
    return home / f"{name}.json"


def errno_for_missing() -> int:
    return errno.ENOENT


def parse_toml(payload: str) -> dict:
    return tomllib.loads(payload)
