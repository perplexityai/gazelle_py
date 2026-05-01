# examples/naming_conventions

Exercises four directives in one workspace:

- `python_library_naming_convention $package_name$_lib`
- `python_test_naming_convention $package_name$_unittest`
- `python_skip_empty_init true`
- `python_test_file_pattern *_spec.py,*_test.py`

Each directive is set on the workspace-root `BUILD.bazel` so the rules they
shape inherit into every package below.

## Layout

```
.
├── BUILD.bazel              # naming + skip-empty-init + custom test patterns
└── widgets/
    ├── empty_only/
    │   └── __init__.py      # empty → no BUILD file generated (skip_empty_init)
    ├── empty_tree/          # project-mode rollup, every src is empty __init__.py
    │   ├── BUILD.bazel      # carries the project directive only — no py_library
    │   ├── __init__.py      # empty
    │   ├── inner/
    │   │   └── __init__.py  # empty
    │   └── leaf/
    │       └── __init__.py  # empty
    ├── relative/
    │   ├── __init__.py      # empty BUT kept in srcs — sibling uses `from . import`
    │   ├── core.py
    │   ├── helpers_local.py # `from . import core`
    │   └── relative_spec.py # → :relative_unittest
    ├── widget/
    │   ├── __init__.py      # non-empty re-exports
    │   ├── widget.py        # → :widget_lib
    │   └── widget_spec.py   # → :widget_unittest (custom test pattern)
    └── helpers/
        ├── __init__.py      # empty — kept in srcs alongside helpers.py
        └── helpers.py       # → :helpers_lib
```

## What this verifies

- `$package_name$` placeholders expand: the rule for `//widgets/widget`
  becomes `:widget_lib` (library) and `:widget_unittest` (test). For
  `//widgets/helpers` the library is `:helpers_lib`.
- `python_skip_empty_init` drops the library that would have been generated
  for `widgets/empty_only/` — its only file is an empty (comments-only)
  `__init__.py`. No `BUILD.bazel` is written there.
- `widgets/relative/` shows the other half of the contract: even with the
  directive on, an empty `__init__.py` stays in `srcs` when siblings exist,
  because `helpers_local.py` does `from . import core` and that relative
  import only resolves when the package marker ships in the same library.
- `widgets/empty_tree/` is the project-mode rollup case: with
  `python_generation_mode project` set, gazelle would normally fold every
  `.py` under that subtree into a single `py_library`. Here all three
  `__init__.py` files are empty, so `python_skip_empty_init` suppresses the
  rollup rule even though there is more than one src.
- `python_test_file_pattern *_spec.py,*_test.py` is a comma-separated list,
  which **replaces** the default patterns. `widget_spec.py` is recognized
  as a test (under defaults it would have been bundled into the library).
  Bare classic `*_test.py` is still allowed because the directive lists
  both forms.

## Try it

```bash
bazel run //:gazelle    # generate / update BUILD files
bazel test //...        # run the renamed unittest rule
bazel run //:gazelle -- update -mode=diff   # idempotency check
```

The BUILD files here are checked in pre-generated; the diff check should
report no changes.
