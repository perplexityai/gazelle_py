from dataclasses import dataclass


@dataclass(frozen=True)
class User:
    name: str
