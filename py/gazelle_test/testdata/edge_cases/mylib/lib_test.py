"""Test-side counterpart: nested imports inside a TestCase method body."""

import unittest

from mylib.lib import Doc, parse_doc


class LibTest(unittest.TestCase):
    def test_parse_doc(self):
        from pathlib import Path  # noqa: F401  in-method nested import

        d = parse_doc("alpha", '{"k": 1}')
        self.assertEqual(d, Doc(name="alpha", body={"k": 1}))


if __name__ == "__main__":
    unittest.main()
