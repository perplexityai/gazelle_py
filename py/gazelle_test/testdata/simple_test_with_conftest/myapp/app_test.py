from myapp.app import greet
from myapp.conftest import NAME


def test_greet() -> None:
    assert greet(NAME) == "hello, world"
