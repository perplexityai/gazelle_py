"""Widget primitive."""

from dataclasses import dataclass


@dataclass(frozen=True)
class Widget:
    name: str
    weight: int
