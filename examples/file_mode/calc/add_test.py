"""Smoke test for file-mode generation.

Imports each sibling library via its own per-file rule. The expected
generated `:add_test` py_test has `deps = [":add", ":helpers", ":mul"]`.
"""

import unittest

from calc.add import add
from calc.helpers import is_finite
from calc.mul import mul


class CalcTest(unittest.TestCase):
    def test_add(self):
        self.assertEqual(add(2, 3), 5)

    def test_mul(self):
        self.assertEqual(mul(4, 3), 12)
        self.assertEqual(mul(0, 99), 0)
        self.assertEqual(mul(-2, 3), -6)

    def test_is_finite(self):
        self.assertTrue(is_finite(1.0))
        self.assertFalse(is_finite(float("inf")))


if __name__ == "__main__":
    unittest.main()
