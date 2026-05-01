from myproj.models.user import User


def render_user(u: User) -> str:
    return f"<{u.name}>"
