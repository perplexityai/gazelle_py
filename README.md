# gazelle_py

A Gazelle language extension for Python, paired with a Rust import-extractor that the plugin links in via cgo.

Built on **Bazel 9 (bzlmod)** with [`rules_rs`](https://github.com/dzbarsky/rules_rs) for the Rust side and `rules_python` for the rules it emits.

## Layout

```
crates/
└── import_extractor/         # Rust staticlib: ruff-based Python import extraction.
                              # Linked into the gazelle plugin via cgo.
proto/                        # Wire format shared by Rust + Go (proto_library).
py/                           # Go-based Gazelle language extension that emits
                              # stock py_library / py_test rules.
platforms/                    # Toolchain platform constraints.
examples/                     # (TBD) self-contained example workspaces.
```

## Architecture

```mermaid
flowchart LR
    subgraph Bazel["Bazel build (gazelle_binary)"]
        direction TB
        gz["gazelle CLI"]
        gobin["gazelle plugin\n(go_library cgo=True)"]
        rsstatic["import_extractor\n(rust_static_library)"]
        gz --> gobin
        gobin -- "cdeps (cgo link)" --> rsstatic
    end

    subgraph Run["At gazelle run time"]
        direction TB
        walk["Gazelle walks BUILD tree"]
        gen["GenerateRules: collect .py files"]
        ffi["ie_dispatch (FFI)"]
        ruff["ruff parser + visitor"]
        resolve["Resolve: imports → deps"]

        walk --> gen
        gen -- "PyQueryRequest (protobuf)" --> ffi
        ffi --> ruff
        ruff -- "PyResponseResult" --> ffi
        ffi --> gen
        gen --> resolve
        resolve --> output["py_library / py_test\nrules + deps"]
    end

    Bazel -. "produced by `bazel build`" .-> Run
```

## What this repo gives you

- **`py`** — Gazelle Python language extension. Generates and maintains `BUILD.bazel` files for Python packages, emitting stock [`py_library`](https://rules-python.readthedocs.io/en/stable/api/rules_python/python/defs.html#py_library) and [`py_test`](https://rules-python.readthedocs.io/en/stable/api/rules_python/python/defs.html#py_test) rules. Consumers swap to their own macros via `# gazelle:map_kind`. Compose your own `gazelle_binary(languages = ["@gazelle_py//py"])`. See [`py/README.md`](py/README.md).
- **`crates/import_extractor`** — Rust staticlib that parses Python imports via [`ruff`](https://github.com/astral-sh/ruff)'s parser. Exposes a 2-function C ABI (`ie_dispatch` / `ie_free`); the gazelle plugin links it via cgo and dispatches in-process — no subprocess startup, no JSON serialization, just protobuf bytes across the FFI boundary. See [`crates/import_extractor/README.md`](crates/import_extractor/README.md).

## Build

```bash
bazel test //...
```

Tests run on Linux x86_64 + macOS arm64 (mirroring [`gazelle_ts`](https://github.com/hermeticbuild/gazelle_ts)).
