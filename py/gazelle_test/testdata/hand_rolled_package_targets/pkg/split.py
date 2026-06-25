import requests


def fetch_status(url: str) -> int:
    return requests.get(url).status_code
