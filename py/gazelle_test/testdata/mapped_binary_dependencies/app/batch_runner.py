import requests


def fetch(url: str) -> None:
    requests.get(url)
