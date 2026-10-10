# Buildkite CI

The native pipeline runs Linux tests, all six examples, Darwin/Windows target
analysis, and PR title/commit checks on the `OSS` cluster's hosted `oss` queue
(8 vCPUs, 32 GB RAM). macOS smoke uses `oss_darwin_arm64` (M4, 12 vCPUs, 56 GB RAM)
and tests `//py:py_test` on main/merge-group builds, matching the GitHub Actions
PR exclusion. Branch builds also run smoke so changes can be tested before merge.
Hook dependencies are installed on every build; titles/commits are checked on PRs.
Bazel 9.0.0 and 8.6.0 run independently. Bazelisk and Node
are checksum-pinned; each test suite and Bazel version uses its own hosted cache
volume. Parallel versions must not share a volume: successful jobs replace its
snapshot rather than merging their cached files.

Buildkite Cache restores the tool, repository, and action caches before Bazel and
saves them after success. `.buildkite/cache.yml` keys entries by pipeline, OS,
architecture, Bazel version, suite, and commit. The commit covers source and
dependency changes without hashing lockfiles that Bazel rewrites during builds.
New commits fall back to the latest entry for the same suite/version; Bazel
checks action inputs.
Hosted agents supply cache storage automatically. Exact restores refresh the
three-day retention; fallback restores do not. Cache archives add transfer time.
The first successful build populates the registry. Check later build logs for
cache restore hits; a normal miss still runs the full build.

Release Please/BCR publishing remain on GitHub Actions. BCR's provenance verifier
checks GitHub attestations from the bazel-contrib release/publish workflows;
running those actions on Buildkite does not preserve that identity. GitHub-managed
CodeQL default setup remains enabled. The native pipeline replaces `ci.yaml` and
`verify-hooks.yml`; `main` requires `buildkite/gazelle-py` from the Buildkite app.

## Pipeline settings

1. In `perplexity/gazelle-py`, use `.buildkite/bootstrap.yml` and the `OSS` cluster.
2. Disable the GitHub Actions pipeline trigger. Enable native GitHub webhook
   processing, pushes to `main`, PR opened/updated/reopened/edited events, and
   merge-group checks. Enable Buildkite commit status reporting. Leave tag
   builds off; releases remain on GitHub Actions.
3. Require `buildkite/gazelle-py` from the Buildkite app in the `main` ruleset.
   PRs run Linux tests/examples and commit validation. Main and merge groups also
   run macOS smoke. Keep Release Please and module-release on GitHub Actions.

The bootstrap must specify `queue: oss`: this cluster's `default` queue is
self-hosted. Use isolated, credential-free agents for contributor PRs. Enable
fork builds only after checking cluster access and approving the intended policy.

PR validation reads public GitHub metadata without a token. GitHub API rate
limits fail the check rather than silently skipping title validation. Builds for
superseded PR heads must be rerun at the current head.

Validate configuration locally:

```sh
bk pipeline validate --file .buildkite/bootstrap.yml --file .buildkite/pipeline.yml
bash -n .buildkite/bazel.sh .buildkite/verify-hooks.sh
```

For a cutover smoke test, open a docs-only PR with a Conventional Commit title.
Confirm the GitHub webhook starts a native Buildkite build with both Bazel
matrices and the PR title/commit check.
