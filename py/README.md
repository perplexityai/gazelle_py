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

The plugin runs through Gazelle's standard three-phase lifecycle. The diagram below traces a single directory's processing:

```mermaid
sequenceDiagram
    autonumber
    participant Gz as gazelle (binary)
    participant Cfg as configure.go
    participant Gen as generate.go
    participant FFI as import_extractor.go (cgo)
    participant Rs as Rust staticlib (ruff)
    participant Idx as RuleIndex
    participant Res as resolve.go

    Gz->>Cfg: Configure(args)
    Note over Cfg: clone parent pyConfig,<br/>apply directives,<br/>seed pyproject deps at root

    Gz->>Gen: GenerateRules(args)
    Gen->>Gen: collectSrcs() → libSrcs / testSrcs
    Gen->>FFI: extractImportsBatch([{abs, rel}…])
    FFI->>Rs: ie_dispatch(PyQueryRequest bytes)
    Rs->>Rs: parse_unchecked + visitor
    Rs-->>FFI: PyResponseResult bytes
    FFI-->>Gen: []FileImports (modules + comments + has_main)
    Gen->>Gen: parseAnnotations(comments)
    Gen-->>Gz: py_library + py_test (with srcs only;<br/>deps not yet set)

    Gz->>Idx: index Imports() specs (pkg, pkg.*, pkg.x for each src)

    Gz->>Res: Resolve(rule, ImportData, from)
    Res->>Res: walk possible-modules ladder<br/>(directives → manifest → RuleIndex → stdlib)
    Res->>Idx: FindRulesByImportWithConfig
    Idx-->>Res: matching labels
    Res->>Res: synthesize ancestor conftest deps (test rules only)
    Res-->>Gz: rule.SetAttr("deps", …)
```

The Rust crate at [`../crates/import_extractor`](../crates/import_extractor) is built as a `rust_static_library` and linked into this `go_library` via `cdeps`. Calls into it go through cgo — no subprocess, no IPC.

## Resolution decision tree

For each import the resolver walks a "possible modules" ladder, trying progressively shorter dotted prefixes (`a.b.c.d` → `a.b.c` → `a.b` → `a`). At each prefix it checks every source in order before stepping shorter — that ordering matters: a single `# gazelle:resolve python <broad> <label>` directive must NOT steal an import that's actually a deeper, more specific submodule provided by another rule.

```mermaid
flowchart TD
    Start([import \"a.b.c.d\"]) --> Skip{relative<br/>or in ignore set?}
    Skip -- yes --> Drop[no dep]
    Skip -- no --> P1[try \"a.b.c.d\"]
    P1 --> P1a{gazelle:resolve<br/>directive?}
    P1a -- yes --> Use[emit dep]
    P1a -- no --> P1b{in manifest?}
    P1b -- yes --> Use
    P1b -- no --> P1c{first-party<br/>RuleIndex hit?}
    P1c -- yes --> Use
    P1c -- no --> P1d{stdlib?}
    P1d -- yes --> Drop
    P1d -- no --> P2[try \"a.b.c\"]
    P2 --> dots[…]
    dots --> Final{any prefix<br/>matched?}
    Final -- yes --> Use
    Final -- no --> Pip[fallback: @pip//&lt;dist&gt;\nif declared in pyproject]
```

## Directives

All directives are placed in `BUILD.bazel` as `# gazelle:<key> <value>` and inherit into subdirectories.

All directive keys mirror [rules_python's gazelle plugin](https://rules-python.readthedocs.io/en/latest/gazelle/docs/index.html) so consumers can swap between the two without rewriting their BUILD-file directives. The one exception is `python_source_extension`, which has no rules_python analog — they hardcode `.py`/`.pyi`.

| Directive | Default | Notes |
|---|---|---|
| `python_extension` | `enabled` | `enabled` / `disabled` (also accepts `true`/`false`). Disable per-tree to skip directories owned by another tool. |
| `python_library_naming_convention` | _(package basename, e.g. `server` for `//apps/server`)_ | Name of the generated library rule. |
| `python_test_naming_convention` | _(package basename + `_test`)_ | Name of the generated test rule. |
| `python_library_kind` | `py_library` | Override emitted library kind without `map_kind`. (Ours; rules_python doesn't have a kind override directive.) |
| `python_test_kind` | `py_test` | Override emitted test kind without `map_kind`. |
| `python_visibility` | `//visibility:public` | Space-separated label list. |
| `python_test_file_pattern` | `*_test.py`, `test_*.py`, `tests/**`, `test/**` | Repeatable; appended. |
| `python_source_extension` | `.py` | Repeatable; appended. (Ours; rules_python hardcodes `.py`/`.pyi`.) |
| `python_label_convention` | `@pip//{pkg}` | Template; `{pkg}` is replaced with the resolved distribution name. |
| `python_test_data` | _(empty)_ | Repeatable; appended to every test rule's `data`. |
| `python_manifest_file_name` | _(empty)_ | Workspace-relative path to a `gazelle_python.yaml` (rules_python format). When set, its `modules_mapping` overrides built-in import → distribution heuristics, and its `pip_repository.name` swaps the repo segment of `python_label_convention`. |

### Per-source-file annotations

The plugin reads `# gazelle:` lines _inside Python source files_ during parsing:

```python
# gazelle:ignore foo,bar          # skip these modules in this file
# gazelle:include_dep //extra:dep # always add this dep to the rule
import foo
import bar
import baz
```

`# gazelle:ignore` accepts either space- or comma-separated module names. The match is prefix-based: ignoring `a.b` covers `a.b.c.D` and the `from` part of `from a.b import x`.

## How import resolution works

1. `pyproject.toml`, `requirements.txt`, and `requirements.in` (if present) are read once at the repo root for declared distribution names.
2. If `python_manifest_file_name` points at a `gazelle_python.yaml`, the file's `modules_mapping` is loaded once on first use.
3. Per import, run the possible-modules ladder shown above.
4. **Test rules** also receive the surrounding library's imports plus synthetic imports for every ancestor directory containing a `conftest.py` — those get resolved like any other import, so a dedicated `:conftest` `py_library` target is automatically picked up while plain `from x.conftest import …` statements (and self-imports) are dropped.

## Running with a custom macro (`map_kind`)

Suppose you want to emit your own `myrepo_py_library` macro instead of stock `py_library`. Add to your root BUILD file:

```starlark
# gazelle:map_kind py_library myrepo_py_library //tools:py.bzl
# gazelle:map_kind py_test    myrepo_py_test    //tools:py.bzl
```

The plugin still emits the stock kinds; gazelle rewrites the kind name and load path on disk. Your macro must accept the attrs the plugin sets (`name`, `srcs`, `deps`, `visibility`, plus `data` and `main` on tests).
