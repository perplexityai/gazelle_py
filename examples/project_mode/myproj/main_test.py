"""Smoke test verifying the rolled-up rule sees every subtree source."""

import unittest

from myproj.main import serialize_user


class ProjectModeTest(unittest.TestCase):
    def test_serialize_user(self):
        self.assertEqual(serialize_user("alice"), '{"rendered": "<alice>"}')


if __name__ == "__main__":
    unittest.main()
