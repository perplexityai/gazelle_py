"""Tiny demo "server" stitching together core types and utils formatting.

Cross-package imports here are what the resolver indexes via the RuleIndex —
each `from packages.<x> ...` line should resolve to its `//packages/<x>` label.
"""

import json

from packages.core.types import Post, User
from packages.utils.format import render_user


def serialize_post(p: Post) -> str:
    return json.dumps(
        {
            "id": p.id,
            "author": render_user(p.author),
            "body": p.body,
        }
    )
