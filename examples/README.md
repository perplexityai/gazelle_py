# examples

Each subdirectory is a self-contained Bazel workspace exercising the `py` plugin against a different scenario.

| Example | What it shows |
|---|---|
| [`basic/`](basic) | One Python package, stdlib-only imports, a smoke test. **No internal cross-package references, no PyPI deps.** Smallest possible useful setup. |
| [`composite/`](composite) | Multiple packages with cross-package imports (`from packages.core.types import ...`). Verifies the resolver's first-party RuleIndex wildcard match. |

Each workspace points its `MODULE.bazel` at the parent `gazelle_py` repo via `local_path_override`, so changes to the plugin's Go source apply on the next `bazel run //:gazelle` without any release dance.

The parent repo's `REPO.bazel` runs `ignore_directories(["examples"])` so `bazel ... //...` walks at the top level skip these — they have their own MODULE files and dependency graphs.

CI runs `bazel test //...` and a `gazelle update -mode=diff` idempotency check in every example.
