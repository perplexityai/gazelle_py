# Buildkite CI

The native pipeline runs Linux tests, all six examples, Darwin/Windows target
analysis, and PR title/commit checks on the `OSS` cluster's hosted `oss` queue
(8 vCPUs, 32 GB RAM). Bazel 9.0.0 and 8.6.0 run independently. Bazelisk and Node
are checksum-pinned; Bazel caches use hosted cache volumes.

macOS smoke and Release Please/BCR publishing remain on GitHub Actions. There is
no macOS queue in this cluster. Existing Linux workflows stay enabled during
cutover because the `main` ruleset requires their GitHub Actions checks.

## Cutover

1. In `perplexity/gazelle-py`, replace the GitHub Actions compatibility importer
   with `.buildkite/bootstrap.yml` and keep the pipeline in the `OSS` cluster.
2. Disable the GitHub Actions pipeline trigger. Enable native GitHub webhook
   processing, pushes to `main`, PR opened/updated/reopened/edited events, and
   merge-group checks. Enable Buildkite commit status reporting. Leave tag
   builds off; releases remain on GitHub Actions.
3. Run this PR through Buildkite. Confirm both Bazel versions, all example checks,
   and commit checks pass. Test a `main` push and merge-group event before cutover.
4. Replace the four GitHub Actions Linux checks in the `main` ruleset with the
   observed Buildkite check. Then remove Linux jobs from `.github/workflows/ci.yaml`
   and retire `.github/workflows/verify-hooks.yml`. Keep the macOS job and release
   workflows.

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
