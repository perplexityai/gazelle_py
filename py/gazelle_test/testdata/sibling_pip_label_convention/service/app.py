import requests


def service_status() -> int:
    return requests.codes.ok
