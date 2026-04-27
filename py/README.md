# py (Gazelle Python language extension)

A Gazelle language extension that generates and maintains BUILD files for Python packages. It emits stock [`rules_python`](https://github.com/bazelbuild/rules_python) `py_library` and `py_test` rules, leaving every project-specific concern (custom macros, pip linker layout, project layout, test runner) configurable via directives or [`# gazelle:map_kind`](https://github.com/bazelbuild/bazel-gazelle#directives).

## Quickstart

Add a `BUILD.bazel` at the repo root with:

```starlark
load("@gazelle//:def.bzl", "gazelle", "gazelle_binary")

gazelle_binary(
    name = "gazelle_bin",
    languages = ["@gazelle_py//py"],
)

gazelle(
    name = "gazelle",
    gazelle = ":gazelle_bin",
)
```

Then run `bazel run //:gazelle`.

`@gazelle_py//py` is a Gazelle Language; you compose your own `gazelle_binary` so it can be combined with other languages (`go`, `ts`, `proto`, …) into a single binary.

By default the plugin emits:

- `py_library` for libraries (loaded from `@rules_python//python:defs.bzl`)
- `py_test` for tests (loaded from `@rules_python//python:defs.bzl`)

If you have your own macros, use `# gazelle:map_kind` to swap.

## Architecture

The plugin runs in three phases per Gazelle's lifecycle:

1. **Configure** ([`configure.go`](configure.go)) — walks the directory tree, applying each directory's BUILD-file directives on top of the inherited config.
2. **GenerateRules** ([`generate.go`](generate.go)) — for each directory, partitions files into source vs test, calls into the Rust staticlib via cgo to extract imports, and emits library + test rules.
3. **Resolve** ([`resolve.go`](resolve.go)) — converts the parsed import statements into Bazel deps using the RuleIndex (for cross-package refs) and the pip link pattern (for PyPI packages).

The Rust crate at [`crates/import_extractor`](../crates/import_extractor) is built as a `rust_static_library` and linked into this `go_library` via `cdeps`. Calls into it go through cgo — no subprocess, no IPC.

## Directives

All directives are placed in `BUILD.bazel` as `# gazelle:<key> <value>` and inherit into subdirectories.

| Directive | Default | Notes |
|---|---|---|
| `py_enabled` | `true` | Disable per-tree to skip directories owned by another tool. |
| `py_library_name` | _(package basename, e.g. `server` for `//apps/server`)_ | Name of the generated library rule. |
| `py_test_name` | _(package basename + `_test`)_ | Name of the generated test rule. |
| `py_library_kind` | `py_library` | Override emitted library kind without `map_kind`. |
| `py_test_kind` | `py_test` | Override emitted test kind without `map_kind`. |
| `py_visibility` | `//visibility:public` | Repeatable / space-separated list. |
| `py_test_pattern` | `*_test.py`, `test_*.py`, `tests/**`, `test/**` | Repeatable; appended. |
| `py_extension` | `.py` | Repeatable; appended. |
| `py_pip_link_pattern` | `@pip//{pkg}` | Template; `{pkg}` is replaced with the resolved distribution name. |
| `py_test_data` | _(empty)_ | Repeatable; appended to every test rule's `data`. |

## How import resolution works

1. `pyproject.toml` and `requirements.txt` (if present) are read once at the repo root for declared distribution names.
2. Per import:
   - **Relative** (`from . import x`, `from .foo import x`): no dep added.
   - **`# gazelle:resolve py <import> <label>` override**: wins over everything else.
   - **Stdlib** (`os`, `sys`, `json`, …): no dep added — the interpreter provides it.
   - **Internal package** (matches a label registered in the RuleIndex): emitted as a workspace-relative label.
   - **PyPI package**: resolves to `{pipLinkPattern}` with `{pkg}` replaced by the lowercased / underscored distribution name.
3. Library and test rules both get `deps`. Tests absorb their own imports plus the surrounding library's imports.

## Running with a custom macro (`map_kind`)

Suppose you want to emit your own `myrepo_py_library` macro instead of stock `py_library`. Add to your root BUILD file:

```starlark
# gazelle:map_kind py_library myrepo_py_library //tools:py.bzl
# gazelle:map_kind py_test    myrepo_py_test    //tools:py.bzl
```

The plugin still emits the stock kinds; gazelle rewrites the kind name and load path on disk. Your macro must accept the attrs the plugin sets (`name`, `srcs`, `deps`, `visibility`, plus `data` and `main` on tests).
