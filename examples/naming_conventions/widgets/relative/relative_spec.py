"""Verifies the relative import inside helpers_local resolves at runtime."""

import unittest

from widgets.relative.helpers_local import doubled_base


class RelativeSpec(unittest.TestCase):
    def test_doubled(self):
        self.assertEqual(doubled_base(), 14)


if __name__ == "__main__":
    unittest.main()
