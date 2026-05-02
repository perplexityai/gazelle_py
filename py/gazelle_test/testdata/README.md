# gazelle_py fixtures

Each subdirectory is one fixture, driven by
`bazel-gazelle/testtools.TestGazelleGenerationOnPath`. Layout:

```
testdata/<fixture>/
├── MODULE.bazel           # marker so gazelle treats this as a repo root
├── BUILD.in               # optional; renamed to BUILD.bazel before gazelle runs
├── BUILD.out              # expected BUILD.bazel after gazelle runs
├── arguments.txt          # optional; newline-delimited gazelle CLI args
├── expectedStdout.txt     # optional
├── expectedStderr.txt     # optional
├── expectedExitCode.txt   # optional; defaults to 0
└── ... .py / subdirs ...  # source tree gazelle should walk
```

Subpackages follow the same convention: a nested `BUILD.in` becomes that
package's `BUILD.bazel`, `BUILD.out` is the per-package golden.

Run a single fixture:

```
bazel test //py/gazelle_test:gazelle_test --test_filter=TestFixtures/<fixture>
```

Update goldens after a deliberate change:

```
UPDATE_SNAPSHOTS=true bazel run //py/gazelle_test:gazelle_test
```

Fixture names mirror rules_python's gazelle/python/testdata where the
behavior overlaps; goldens reflect this plugin's output (e.g.
`load("@rules_python//python:defs.bzl", ...)`, `//visibility:public` default,
no `main` attr on `py_test`).

The `composite`, `edge_cases`, `project_mode`, `relative_imports`, and
`skip_empty_init_project` fixtures mirror the corresponding example
workspaces under `//examples/`. Keeping a hermetic copy in testdata means
the harness exercises the same setups as the examples (which are out-of-tree
workspaces driven by `bazel run //:gazelle` rather than golden compares),
catching gazelle regressions before they reach `examples/`.
