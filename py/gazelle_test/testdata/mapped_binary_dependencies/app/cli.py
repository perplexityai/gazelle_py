import requests

from app.helper import normalize


def main() -> None:
    requests.get(normalize("https://example.com"))
