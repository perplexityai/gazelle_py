# examples/project_mode

Exercises `# gazelle:python_generation_mode project` — every `.py` file under
the directive's directory is rolled up into a single `py_library` (and a
single `py_test`), and subdirectories produce no rules of their own.

## Layout

```
.
└── myproj/
    ├── BUILD.bazel        # # gazelle:python_generation_mode project
    ├── main.py            # rolled in
    ├── main_test.py       # → :myproj_test
    ├── models/
    │   └── user.py        # rolled in (no BUILD file here)
    └── utils/
        └── format.py      # rolled in (no BUILD file here)
```

## What this verifies

- The directive at `myproj/BUILD.bazel` rolls up every `.py` under
  `myproj/` into one `:myproj` library — `srcs` lists `main.py`,
  `models/user.py`, and `utils/format.py`.
- Subdirectories (`models/`, `utils/`) **do not** get their own BUILD files.
  Bazel treats `myproj/` as one package and the cross-directory `srcs` only
  work because no nested BUILD file marks a sub-package boundary. If you
  adopt project mode in an existing tree you must clear pre-existing
  `BUILD.bazel` files in the subtree first.
- Imports between the rolled-up files (`main.py` → `myproj.models.user`,
  `utils.format` → `myproj.models.user`) resolve to the same library, so the
  generated rule has no self-dep on `:myproj`.
- The single test file in the project root becomes `:myproj_test` with
  `deps = [":myproj"]`, picking up everything the library brings in.

## Try it

```bash
bazel run //:gazelle    # generate / update BUILD files
bazel test //...        # run the rolled-up tests
bazel run //:gazelle -- update -mode=diff   # idempotency check
```

The BUILD files here are checked in pre-generated; the diff check should
report no changes.
