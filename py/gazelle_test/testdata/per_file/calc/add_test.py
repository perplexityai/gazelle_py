from calc.add import add
from calc.mul import mul


def test_add() -> None:
    assert add(1, 2) == 3


def test_mul() -> None:
    assert mul(2, 3) == 6
