"""User model — rolled into //myproj:myproj alongside main.py."""

from dataclasses import dataclass


@dataclass(frozen=True)
class User:
    name: str
