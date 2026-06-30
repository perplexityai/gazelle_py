from app.openapi._bootstrap import ensure_test_tmpdir


def test_tmpdir() -> None:
    assert ensure_test_tmpdir()
