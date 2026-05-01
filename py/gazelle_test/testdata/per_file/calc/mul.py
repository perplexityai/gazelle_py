from calc.add import add


def mul(a: int, b: int) -> int:
    out = 0
    for _ in range(b):
        out = add(out, a)
    return out
