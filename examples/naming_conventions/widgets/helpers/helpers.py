"""Stdlib-only helper used by the widget tests via include_dep."""

import math


def round_weight(weight: float) -> int:
    return math.floor(weight + 0.5)
