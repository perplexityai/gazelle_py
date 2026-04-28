"""Tests + a few more nested-import shapes for the resolver to walk.

The library exports stdlib-only behavior; this file just exercises it.
We add a couple of test-only nested imports so the test rule itself
covers the same recursion paths.
"""

import unittest

from mylib.lib import Doc, doc_path, errno_for_missing, is_modern, lazy_quote, parse_doc


class LibTest(unittest.TestCase):
    def test_parse_doc(self):
        # Function-body import inside a test method.
        from pathlib import Path  # noqa: F401  ensures nested imports compile

        d = parse_doc("alpha", '{"k": 1}')
        self.assertEqual(d, Doc(name="alpha", body={"k": 1}))

    def test_lazy_quote(self):
        self.assertEqual(lazy_quote("a b"), "a%20b")

    def test_errno_for_missing(self):
        # `errno.ENOENT` is platform-defined but always present.
        self.assertGreater(errno_for_missing(), 0)

    def test_is_modern(self):
        # We pin python_version = "3.12" in MODULE.bazel, so the modern
        # branch (3.11+) is what runs.
        self.assertTrue(is_modern())

    def test_doc_path(self):
        from pathlib import Path

        self.assertEqual(doc_path(Path("/tmp"), "alpha"), Path("/tmp/alpha.json"))


if __name__ == "__main__":
    unittest.main()
