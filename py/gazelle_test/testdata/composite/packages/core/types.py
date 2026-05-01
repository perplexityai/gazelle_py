from dataclasses import dataclass


@dataclass(frozen=True)
class User:
    id: int
    name: str
    email: str


@dataclass(frozen=True)
class Post:
    id: int
    author: User
    body: str
