"""Formatting helpers — depends on `core` for the User type."""

from packages.core.types import User


def render_user(u: User) -> str:
    return f"{u.name} <{u.email}>"
