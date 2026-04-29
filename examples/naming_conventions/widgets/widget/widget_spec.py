"""Test file using the alternate `*_spec.py` pattern.

Discovered as a test only because the workspace's
`# gazelle:python_test_file_pattern *_spec.py,*_test.py` directive
replaces the default test-pattern list. Without the override, this file
would be glued onto the library rule instead.
"""

import unittest

from widgets.widget import Widget


class WidgetSpec(unittest.TestCase):
    def test_construction(self):
        w = Widget(name="bolt", weight=4)
        self.assertEqual(w.name, "bolt")
        self.assertEqual(w.weight, 4)


if __name__ == "__main__":
    unittest.main()
