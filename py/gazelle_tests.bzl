"""gazelle_tests: drive directory-based fixture tests for the gazelle_py plugin.

Each fixture is a directory under `testdata/` containing:

  - the source tree gazelle should walk (.py files, subdirs, …)
  - one `BUILD.in` per directory that should start with seed BUILD content
    (omit when gazelle should generate from scratch)
  - one `BUILD.out` per directory describing the expected post-gazelle BUILD
  - optional `arguments.txt`, `expectedStdout.txt`, `expectedStderr.txt`,
    `expectedExitCode.txt` (the convention used by
    `bazel-gazelle/testtools.TestGazelleGenerationOnPath`)

Inspired by rules_python's gazelle/python/testdata fixtures, but driven by the
upstream gazelle testtools harness rather than a hand-rolled YAML reader.
"""

load("@gazelle//:def.bzl", "gazelle_binary")
load("@rules_go//go:def.bzl", "go_test")

def gazelle_tests(
        name,
        srcs,
        testdata,
        languages = None,
        size = "medium",
        **kwargs):
    """Wraps a gazelle_binary + go_test that walks fixture directories.

    Args:
      name: name of the resulting go_test target.
      srcs: Go test source(s). Must call testtools.TestGazelleGenerationOnPath
        with the binary path passed via the `-gazelle_binary` flag and resolve
        each fixture under the runfiles path returned by `runfiles.Rlocation`.
      testdata: list of files (typically `glob(["testdata/**"])`).
      languages: list of gazelle Language go_library labels. Defaults to
        ["//py"], which is the right answer when this macro is used inside the
        gazelle_py repo itself; override for downstream consumers.
      size: bazel test size attribute.
      **kwargs: forwarded to go_test (e.g. deps, embed, env).
    """
    if languages == None:
        languages = ["//py"]

    binary_name = name + "_gazelle_bin"
    gazelle_binary(
        name = binary_name,
        languages = languages,
        testonly = True,
    )

    go_test(
        name = name,
        srcs = srcs,
        size = size,
        args = ["-gazelle_binary=$(rlocationpath :%s)" % binary_name],
        data = [":" + binary_name] + testdata,
        **kwargs
    )
