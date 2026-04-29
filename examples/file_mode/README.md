# examples/file_mode

Exercises `# gazelle:python_generation_mode file` — one rule per source
file instead of one rule per directory.

## Layout

```
.
└── calc/
    ├── BUILD.bazel    # # gazelle:python_generation_mode file
    ├── add.py         # → py_library(name = "add")
    ├── helpers.py     # → py_library(name = "helpers")
    ├── mul.py         # → py_library(name = "mul", deps = [":add"])
    └── add_test.py    # → py_test(name = "add_test", deps = [":add", ":helpers", ":mul"])
```

## What this verifies

- The directive is read on the **package** (not the workspace root), so the
  switch is local to `//calc` and other packages keep package-mode behavior.
- Per-file libraries are named after the file basename (`add.py` → `:add`,
  `mul.py` → `:mul`).
- A per-file library can depend on a sibling per-file library — `mul.py`
  imports `from calc.add import add`, and the resolver fills in `deps = [":add"]`
  via the RuleIndex.
- Test files emit one `py_test` per file. The test's `deps` aggregate every
  sibling library's import set, since file mode keeps the surrounding
  library's deps reachable from each test target.

## Try it

```bash
bazel run //:gazelle    # generate / update BUILD files
bazel test //...        # run the per-file tests
bazel run //:gazelle -- update -mode=diff   # idempotency check
```

The `BUILD.bazel` here is checked in pre-generated; the diff check should
report no changes.
