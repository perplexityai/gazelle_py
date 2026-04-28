"""Smoke test — imports the sibling library via its package-qualified path.

The resolver should add `:myapp` (the library) to this rule's deps via the
RuleIndex first-party path (`myapp.*` wildcard match).
"""

import json
import tempfile
import unittest
from pathlib import Path

from myapp.app import Config, load_config


class LoadConfigTest(unittest.TestCase):
    def test_roundtrip(self):
        with tempfile.NamedTemporaryFile("w", suffix=".json", delete=False) as f:
            json.dump({"name": "demo", "home": "/tmp"}, f)
            path = f.name

        cfg = load_config(path)
        self.assertEqual(cfg.name, "demo")
        self.assertEqual(cfg.home, Path("/tmp"))


if __name__ == "__main__":
    unittest.main()
