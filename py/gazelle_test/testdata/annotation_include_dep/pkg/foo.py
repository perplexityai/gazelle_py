# gazelle:include_dep //extra:always_needed
# gazelle:include_dep @some_repo//pkg:lib

# This file imports nothing third-party — but the include_dep annotations
# above still produce deps on the generated rule. Useful for runtime-only
# data deps the static analyzer can't see (e.g. resources loaded by
# importlib.resources).


def foo() -> str:
    return "foo"
