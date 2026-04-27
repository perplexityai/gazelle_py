# gazelle_py

Bazel build setup, a Gazelle Python language extension, and the Rust import-extractor that powers it (linked in via cgo).

Built on **Bazel 9 (bzlmod)** with [`rules_rs`](https://github.com/dzbarsky/rules_rs) for the Rust side and `rules_python` for the Python examples.

## Layout

```
crates/
└── import_extractor/         # Rust staticlib: Python import extraction.
                              # Linked into the gazelle plugin via cgo.
py/                           # Go-based Gazelle language extension that emits
                              # stock py_library / py_test rules
examples/                     # self-contained Bazel workspaces (TBD)
```

## What this repo gives you

- **`py`** — Gazelle Python Language extension. Generates and maintains `BUILD.bazel` files for Python packages, emitting stock `py_library` and `py_test` rules; consumers swap to their own macros via `# gazelle:map_kind`. Consume by composing your own `gazelle_binary(languages = ["@gazelle_py//py"])`. See [`py/README.md`](py/README.md).
- **`crates/import_extractor`** — Rust staticlib that parses Python imports. Exposes a small C ABI (`ie_dispatch` / `ie_free`); the gazelle plugin links it via cgo and dispatches in-process. See [`crates/import_extractor/README.md`](crates/import_extractor/README.md).

## Build

```
bazel test //...
```
