import requests
import undeclared_package


def names() -> tuple[str, str]:
    return requests.__name__, undeclared_package.__name__
