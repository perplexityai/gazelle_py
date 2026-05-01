"""Imports a handful of stdlib modules. The plugin must NOT add deps for
these — they're shipped with the interpreter."""

import json
import os
import os.path  # nested stdlib import
from collections import OrderedDict
from typing import Any


def fingerprint(env: dict[str, Any]) -> str:
    items = OrderedDict(sorted(env.items()))
    return json.dumps(items, separators=(",", ":")) + os.path.sep
