"""Unmanaged third-party imports. With no manifest and no first-party rule
that owns these modules, gazelle_py falls back to its `python_label_convention`
template (default `@pip//{pkg}`) to construct deps."""

import requests
from numpy import array


def fetch(url: str) -> list[float]:
    resp = requests.get(url)
    resp.raise_for_status()
    return list(array(resp.json(), dtype=float))
