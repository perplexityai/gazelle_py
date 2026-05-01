import json

from packages.core.types import Post, User
from packages.utils.format import render_user


def serialize_post(p: Post) -> str:
    return json.dumps(
        {
            "id": p.id,
            "author": render_user(p.author),
            "body": p.body,
        }
    )


def make_user(id_: int, name: str, email: str) -> User:
    return User(id=id_, name=name, email=email)
