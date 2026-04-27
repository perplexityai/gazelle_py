# import_extractor

Rust staticlib that extracts import paths from Python source files. Linked into the gazelle plugin's `go_library` via cgo and dispatched in-process — no subprocess, no IPC.

## Why a Rust crate

Parsing Python correctly enough to drive `BUILD.bazel` generation is significantly easier in Rust than Go: [`rustpython-parser`](https://crates.io/crates/rustpython-parser) produces a real AST and is fast enough for the gazelle hot path. The crate parses files in parallel via `rayon`.

## C ABI

Two functions, declared in `src/ffi.rs`:

```c
void ie_dispatch(
    const uint8_t *req_ptr,
    size_t req_len,
    uint8_t **out_resp_ptr,
    size_t *out_resp_len);

void ie_free(uint8_t *ptr, size_t len);
```

`ie_dispatch` decodes a protobuf `Request`, parses the requested files in parallel, encodes a `Response`, and hands ownership of the buffer back via the out-parameters. The caller releases it with `ie_free`. The encoding is the same protobuf schema in [`proto/message.proto`](../../proto/message.proto) — it just runs in-process now.

## Layout

```
proto/
└── message.proto       # wire-protocol schema (built via rust_prost_library)
src/
├── lib.rs              # re-exports ffi, py, wire modules
├── ffi.rs              # C ABI surface (ie_dispatch / ie_free)
├── wire.rs             # protobuf request/response dispatcher
└── py.rs               # rustpython-parser-based Python import extractor
```

## Build

```
bazel build //crates/import_extractor:import_extractor_static
bazel test  //crates/import_extractor:test
```

The `import_extractor_static` target produces a `.a` with `CcInfo`, which `//py:py` consumes via `cdeps` on its `go_library`.

## Performance notes

The workspace `[profile.release]` sets `panic = "abort"` and `codegen-units = 1`, and Bazel's `--config=opt` mirrors them via `@rules_rust//:extra_rustc_flags=-Ccodegen-units=1,-Cpanic=abort,-Cstrip=symbols,-Clto=thin`. Both meaningfully help the gazelle plugin's hot path — calls into the FFI run on every directory Gazelle visits.
