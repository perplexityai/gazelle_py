"""Re-exports widget.Widget so callers can `from widgets.widget import Widget`.

This file is non-empty — it has real code — so even with
`python_skip_empty_init = true` the library is preserved.
"""

from widgets.widget.widget import Widget

__all__ = ["Widget"]
