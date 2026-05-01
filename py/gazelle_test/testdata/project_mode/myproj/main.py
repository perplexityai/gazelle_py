import json

from myproj.models.user import User
from myproj.utils.format import render_user


def serialize_user(name: str) -> str:
    return json.dumps({"rendered": render_user(User(name=name))})
