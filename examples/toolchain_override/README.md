# examples/toolchain_override

Demonstrates and regression-tests the pattern for overriding the Rust toolchain version that `gazelle_py` propagates to its consumers.

## What this guards against

`gazelle_py`'s `MODULE.bazel` pins `rules_rs`'s toolchain to `1.92.0` (matching ruff 0.15.10's MSRV) and registers it for downstream builds. Consumers may need a different version — typically newer, e.g. fleet-wide pin already on `1.93.0`+ — without forking the plugin.

The override pattern, exercised here, is:

1. In your root `MODULE.bazel`, call `toolchains.toolchain(name = "<custom_name>", version = "<your_version>", ...)` with a **custom name**. Reusing rules_rs's default `default_rust_toolchains` would collide with `gazelle_py`'s tag and fail module resolution with `"Toolchain repo default_rust_toolchains has conflicting tag configurations"`.
2. Call `register_toolchains("@<custom_name>//:all")`. Bazel evaluates root-module registrations before deps', so yours wins over `gazelle_py`'s `register_toolchains("@default_rust_toolchains//:all")`.

This example pins `consumer_rust_toolchains@1.93.0` and asserts via `sh_test` that `bazel`-resolved `rustc --version` reports `1.93.0` rather than `gazelle_py`'s `1.92.0`. If the override mechanism ever regresses — rules_rs changes tag-merge order, registration precedence shifts, etc. — the assertion fails loudly instead of silently falling back.

## Try it

```bash
bazel test //...                           # resolves toolchain, builds gazelle_bin, asserts version
bazel build //:active_rustc_version        # writes the version string only
cat bazel-bin/active_rustc_version.txt
```

The `gazelle_bin` target also builds `@gazelle_py//crates/import_extractor:import_extractor_static` transitively via cgo, so a successful build also verifies that `gazelle_py`'s staticlib compiles cleanly under the consumer's toolchain.
