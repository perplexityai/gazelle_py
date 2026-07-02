from google.api_core import retry
from PIL import Image


def names() -> tuple[str, str]:
    return retry.__name__, Image.__name__
