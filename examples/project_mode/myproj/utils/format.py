"""Formatting helpers — also rolled into //myproj:myproj. The dotted import
of `myproj.models.user` resolves to the same library, so it doesn't show
up as an explicit dep on the generated rule.
"""

from myproj.models.user import User


def render_user(u: User) -> str:
    return f"<{u.name}>"
