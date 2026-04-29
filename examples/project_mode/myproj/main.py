"""Entry point for the rolled-up project.

Imports its sibling subpackages — under `python_generation_mode project`
they all live in the same `:myproj` py_library, so these are local imports
that don't need any explicit Bazel deps.
"""

import json

from myproj.models.user import User
from myproj.utils.format import render_user


def serialize_user(name: str) -> str:
    return json.dumps({"rendered": render_user(User(name=name))})
