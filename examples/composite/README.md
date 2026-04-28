# examples/composite

Multi-package layout exercising the plugin's first-party RuleIndex resolution.

## Layout

```
.
├── apps/web/
│   ├── server.py        # imports packages.core.types + packages.utils.format
│   └── server_test.py   # imports apps.web.server + packages.core.types
└── packages/
    ├── core/
    │   └── types.py     # leaf — stdlib only
    └── utils/
        └── format.py    # imports packages.core.types
```

## What this verifies

- Each directory gets its own `py_library` (or `py_test`) named after the directory basename.
- Cross-package imports of the form `from packages.<name>.<module> import …` resolve to the matching `//packages/<name>` label via the resolver's wildcard index path.
- A test rule's `deps` includes the sibling library plus every cross-package import; gazelle auto-fills both.

## Try it

```bash
bazel run //:gazelle    # generate/update BUILD files
bazel test //...        # run the smoke test
```

The per-package `BUILD.bazel` files are checked in pre-generated; running `bazel run //:gazelle -- update -mode=diff` should report no changes.
