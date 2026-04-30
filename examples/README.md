# examples

Each subdirectory is a self-contained Bazel workspace exercising the `py` plugin against a different scenario.

| Example | What it shows |
|---|---|
| [`basic/`](basic) | One Python package, stdlib-only imports, a smoke test. **No internal cross-package references, no PyPI deps.** Smallest possible useful setup. |
| [`composite/`](composite) | Multiple packages with cross-package imports (`from packages.core.types import ...`). Verifies the resolver's first-party RuleIndex wildcard match. |
| [`edge_cases/`](edge_cases) | Imports nested inside every kind of code block — function bodies, class bodies, `if TYPE_CHECKING:`, `try` / `except ImportError` fallbacks, conditional `if sys.version_info` branches, multi-target `import a, b`. Regression net for the ruff visitor's recursion. |
| [`file_mode/`](file_mode) | `# gazelle:python_generation_mode file` — one rule per source file. Sibling per-file libraries depend on each other through the RuleIndex. |
| [`project_mode/`](project_mode) | `# gazelle:python_generation_mode project` — every `.py` under one directory rolls into a single library/test. Subdirectories must not have BUILD files. |
| [`naming_conventions/`](naming_conventions) | `python_library_naming_convention` / `python_test_naming_convention` with the `$package_name$` placeholder, `python_skip_empty_init`, and `python_test_file_pattern` comma-list replacement. |
| [`toolchain_override/`](toolchain_override) | Consumer overrides `gazelle_py`'s pinned Rust toolchain by registering its own `toolchains.toolchain(...)` under a custom name. **No Python source.** `sh_test` asserts the consumer's `rustc --version` wins toolchain resolution. |

Each workspace points its `MODULE.bazel` at the parent `gazelle_py` repo via `local_path_override`, so changes to the plugin's Go source apply on the next `bazel run //:gazelle` without any release dance.

The parent repo's `REPO.bazel` runs `ignore_directories(["examples"])` so `bazel ... //...` walks at the top level skip these — they have their own MODULE files and dependency graphs.

CI runs `bazel test //...` and a `gazelle update -mode=diff` idempotency check in every example.
