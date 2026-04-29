"""Multiplication primitive. Imports its sibling `add` to demonstrate that
file-mode rules can depend on each other through the in-package RuleIndex.
"""

from calc.add import add


def mul(a: int, b: int) -> int:
    if b == 0:
        return 0
    result = 0
    sign = 1 if (a >= 0) == (b >= 0) else -1
    a, b = abs(a), abs(b)
    for _ in range(b):
        result = add(result, a)
    return sign * result
