from acme.common.utils import normalize
from src.tools.tool import render


def run(value: str) -> str:
    return render(normalize(value))
