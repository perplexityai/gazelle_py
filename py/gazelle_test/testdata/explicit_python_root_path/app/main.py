from projects.tools.tool import render
from runtime.common.utils import normalize


def run(value: str) -> str:
    return render(normalize(value))
