"""Pure stdlib helper. With `python_generation_mode file` this lands in its
own `:helpers` library, separately depended on by callers that need it.
"""

import math


def is_finite(x: float) -> bool:
    return math.isfinite(x)
