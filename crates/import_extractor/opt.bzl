"""rust_static_library wrapper that pins compilation_mode and rustc flags.

The gazelle plugin's go_library links the import_extractor staticlib via
cgo. Without a transition, the staticlib's build flags would inherit the
consumer's compilation_mode, so a consumer running plain `bazel build`
would get a fastbuild rust binary inside their gazelle binary.

with_cfg forces compilation_mode=opt and the release rustc flags
(panic=abort, codegen-units=1, thin LTO, strip) on the staticlib and its
transitive deps. The transition propagates so @gazelle_py_crates//
crates are optimized too.
"""

load("@rules_rs//rs:rust_static_library.bzl", "rust_static_library")
load("@with_cfg.bzl", "with_cfg")

opt_rust_static_library, _opt_rust_static_library_internal = (
    with_cfg(rust_static_library)
        .set("compilation_mode", "opt")
        .set(
            Label("@rules_rust//:extra_rustc_flags"),
            [
                "-Ccodegen-units=1",
                "-Cpanic=abort",
                "-Cstrip=symbols",
                "-Clto=thin",
                "-Cembed-bitcode=yes",
            ],
        )
        .build()
)
