from ai_training.common.utils import normalize
from data.scripts.tool import render


def run(value: str) -> str:
    return render(normalize(value))
