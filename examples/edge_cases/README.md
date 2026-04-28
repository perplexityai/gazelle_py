# examples/edge_cases

Exercises every kind of code block the ruff-based extractor recurses into. All imports here are stdlib (or behind a `# gazelle:ignore`), so the resolved `deps` on the generated `py_library` and `py_test` should both be empty.

## What this verifies

- **Module-level**: `import json`, `from pathlib import Path`, aliased (`import dataclasses as dc`), and multi-target (`import os, errno`).
- **`if TYPE_CHECKING:` block**: `from collections.abc import Iterable` — only seen at type-check time, but the parser walks the block and we still drop `collections.abc` (stdlib).
- **Function body**: `import urllib.parse` inside `lazy_quote()` — the classic circular-import workaround.
- **Class body**: `import functools` inside a class — uncommon but legal Python.
- **`try` / `except ImportError` fallback**: `import tomllib` with `import tomli as tomllib` in the except branch. The fallback is `# gazelle:ignore`d so the resolver doesn't emit `@pip//tomli` (we don't declare it in pyproject.toml; the fallback only fires on Python <3.11).
- **Module-scope conditional**: `if sys.version_info >= (3, 11): from datetime import UTC` — both branches are walked.
- **Test-method-body imports**: `from pathlib import Path` inside a `unittest.TestCase` method.

## Try it

```bash
bazel run //:gazelle    # generate / update BUILD files
bazel test //...        # run the smoke tests
bazel run //:gazelle -- update -mode=diff   # idempotency check
```

If gazelle starts emitting bogus `@pip//<stdlib>` deps because it missed a stdlib check on a nested import, the idempotency check fails — that's the regression signal.
