"""Uses `from . import core` — a relative import that requires the package's
`__init__.py` to ship in the same py_library as this file. The
`python_skip_empty_init` directive must keep that __init__.py in srcs because
real siblings exist here.
"""

from . import core


def doubled_base() -> int:
    return core.base_value() * 2
