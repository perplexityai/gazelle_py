# Buildkite CI

The native pipeline runs Linux tests, all six examples, Darwin/Windows target
analysis, and PR title/commit checks on the `OSS` cluster's hosted `oss` queue
(8 vCPUs, 32 GB RAM). macOS smoke uses `oss_darwin_arm64` (M4, 12 vCPUs, 56 GB RAM)
and tests `//py:py_test` on main/merge-group builds, matching the GitHub Actions
PR exclusion. Branch builds also run smoke so changes can be tested before merge.
Hook dependencies are installed on every build; titles/commits are checked on PRs.
The post-checkout hook tests GitHub's PR merge ref, matching `actions/checkout`.
It rejects stale merge refs that do not contain the expected PR head.
Bootstrap pins that merge SHA in build metadata so every job tests the same tree.
The trusted bootstrap skips repository checkout and checks GitHub PR metadata
first. Forks require a pipeline writer to approve that commit before any fork
checkout/hooks. Only GitHub's `renovate[bot]` identity (ID `29139614`, type `Bot`)
bypasses fork approval. New commits get new builds and approvals; API errors or
stale PR heads fail closed.
Bazel 9.0.0 and 8.6.0 run independently. Bazelisk and Node are checksum-pinned.

## Cache isolation

Bazel jobs use the `oss-ci-v2` registry with server-enforced pipeline/branch scopes.
PRs restore their own branch or main; writes stay in their verified branch.
Main restores only main. GitHub **Prefix third-party fork branch names** must stay
on, so a fork branch named `main` cannot write the real main scope. Cache keys and
PR-controlled YAML are not access controls.

Shared hosted cache volumes are removed. Only repository/action caches persist;
Bazelisk and downloaded tools start fresh per job. The new registry starts empty,
so existing untrusted cache entries are never restored. Initial builds run cold.

Create the registry once in the OSS cluster with `.buildkite/cache-policy.json`.
Do not replace it with the unrestricted default registry. Policy scopes come from
Buildkite's authenticated job claims, not environment variables supplied by jobs.

```mermaid
flowchart LR
  M[Main job] -->|save / restore| MC[Main cache scope]
  P[PR job] -->|save / restore| PC[PR branch cache scope]
  MC -->|read only| P
```

Release Please/BCR publishing remain on GitHub Actions. BCR's provenance verifier
checks GitHub attestations from the bazel-contrib release/publish workflows;
running those actions on Buildkite does not preserve that identity. GitHub-managed
CodeQL default setup remains enabled. The native pipeline replaces `ci.yaml` and
`verify-hooks.yml`; `main` requires `buildkite/gazelle-py` from the Buildkite app.

## Pipeline settings

1. In `perplexity/gazelle-py`, paste `.buildkite/bootstrap.yml` into pipeline
   settings and use the `OSS` cluster. The first step must be inline there;
   loading a bootstrap from the PR checkout would let fork code bypass approval.
2. Disable the GitHub Actions pipeline trigger. Enable native GitHub webhook
   processing, pushes to `main`, PR opened/updated/reopened/edited events, and
   merge-group checks. Enable Buildkite commit status reporting. Leave tag
   builds off; releases remain on GitHub Actions. Set blocked build statuses to
   Pending. Enable third-party fork builds only after installing this bootstrap.
3. Require `buildkite/gazelle-py` from the Buildkite app in the `main` ruleset.
   PRs run Linux tests/examples and commit validation. Main and merge groups also
   run macOS smoke. Keep Release Please and module-release on GitHub Actions.

The bootstrap must specify `queue: oss`: this cluster's `default` queue is
self-hosted. Use isolated, credential-free agents for contributor PRs. Enable
fork builds with the trusted approval bootstrap above. Keep workflow access
tokens disabled and pipeline/cluster secrets unavailable to these jobs.

PR validation reads public GitHub metadata without a token. GitHub API rate
limits fail the check rather than silently skipping title validation. Builds for
superseded PR heads must be rerun at the current head.

Validate configuration locally:

```sh
bk pipeline validate --file .buildkite/bootstrap.yml --file .buildkite/pipeline.yml
bash -n .buildkite/bazel.sh .buildkite/verify-hooks.sh .buildkite/hooks/post-checkout
```

For a cutover smoke test, open a docs-only PR with a Conventional Commit title.
Confirm the GitHub webhook starts a native Buildkite build with both Bazel
matrices and the PR title/commit check.


## Releases

GHA waits for `release-ready/gazelle-py` on the exact release tag commit. Only non-PR main builds publish that status, after the full BK pipeline finishes. Failed builds stop publication; missing/pending checks time out after 90 minutes. No new secrets or BK token permissions.

The reusable GHA release workflow checks that its checkout still matches the verified SHA, packages the source archive, attests provenance, and publishes BCR. Its duplicate Bazel test run and Bazel caches are disabled. Existing runner exceptions remain on GHA.

```mermaid
flowchart TD
  Main[Main commit] --> BK[BK full test pipeline]
  BK --> Status[Main-only release-ready status]
  Tag[Release tag] --> Gate[GHA checks exact SHA, main ancestry, BK success]
  Status --> Gate
  Gate --> Archive[GHA rechecks SHA and packages source]
  Archive --> Provenance[GHA provenance]
  Provenance --> BCR[GitHub release and BCR]
```
