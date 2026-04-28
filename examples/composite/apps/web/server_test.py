"""Server smoke test — imports the sibling library plus core types."""

import json
import unittest

from apps.web.server import serialize_post
from packages.core.types import Post, User


class SerializePostTest(unittest.TestCase):
    def test_renders_author(self):
        u = User(id=1, name="Ada", email="ada@example.com")
        p = Post(id=42, author=u, body="hello")
        out = json.loads(serialize_post(p))
        self.assertEqual(out["id"], 42)
        self.assertEqual(out["author"], "Ada <ada@example.com>")


if __name__ == "__main__":
    unittest.main()
