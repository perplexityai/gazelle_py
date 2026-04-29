# bcr_test/

Minimal consumer-side smoke workspace used by the BCR presubmit
([`.bcr/presubmit.yml`](../.bcr/presubmit.yml)) — and only by it.

Why this exists separately from [`examples/`](../examples):

- `examples/` are user-facing demos. They're cohesive packages a reader
  picks up to learn how to use the plugin, plus richer scenarios
  (composite layouts, edge-case imports, generation modes, naming).
- `bcr_test/` is purpose-built for BCR's CI sandbox. It exercises the
  *published* module's targets through `local_path_override` and pins the
  Linux host platform via its own `.bazelrc` — BCR's sandbox doesn't read
  the top-level `.bazelrc`, so building `@gazelle_py//...` directly from
  BCR fails on Ubuntu without the pin. Mixing this concern into examples/
  would force the user-facing demos to carry the BCR-specific override.

This directory is added to [`REPO.bazel`](../REPO.bazel)'s
`ignore_directories` list so the parent module's `bazel ... //...` walks
skip it.

## Manual run

```bash
cd bcr_test
bazel build @gazelle_py//py/... @gazelle_py//crates/import_extractor/...
bazel test  @gazelle_py//py:py_test @gazelle_py//crates/import_extractor:test
```
