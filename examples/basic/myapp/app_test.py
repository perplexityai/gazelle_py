"""Smoke test — imports the sibling library via its package-qualified path.

The resolver should add `:myapp` (the library) to this rule's deps via the
RuleIndex first-party path (`myapp.*` wildcard match), and `:conftest` via
the ancestor-conftest synthesis (resolve.go's `conftestImportsFor`).
"""

import unittest
from pathlib import Path

from myapp.app import load_config
from myapp.conftest import write_config_file


class LoadConfigTest(unittest.TestCase):
    def test_roundtrip(self):
        path = write_config_file(name="demo", home="/tmp")
        cfg = load_config(str(path))
        self.assertEqual(cfg.name, "demo")
        self.assertEqual(cfg.home, Path("/tmp"))


if __name__ == "__main__":
    unittest.main()
