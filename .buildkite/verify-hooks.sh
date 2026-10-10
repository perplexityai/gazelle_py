#!/usr/bin/env bash
set -euo pipefail

work_dir=$(mktemp -d)
trap 'rm -rf "$work_dir"' EXIT
curl --fail --silent --show-error --location --retry 3 \
  https://nodejs.org/dist/v24.12.0/node-v24.12.0-linux-x64.tar.xz \
  --output "$work_dir/node.tar.xz"
echo "bdebee276e58d0ef5448f3d5ac12c67daa963dd5e0a9bb621a53d1cefbc852fd  $work_dir/node.tar.xz" | sha256sum --check
tar -xJf "$work_dir/node.tar.xz" -C "$work_dir"
export PATH="$work_dir/node-v24.12.0-linux-x64/bin:$PATH"

npm install --global --prefix "$work_dir/pnpm" pnpm@11.17.0
export PATH="$work_dir/pnpm/bin:$PATH"
pnpm install --frozen-lockfile

# GitHub Actions also installs hook dependencies on main and merge-group builds.
if [[ "${BUILDKITE_PULL_REQUEST:-false}" == false ]]; then
  exit
fi
[[ "${BUILDKITE_PULL_REQUEST:-}" =~ ^[0-9]+$ ]] || {
  echo 'Expected a pull request number' >&2
  exit 1
}

# Public repository metadata needs no GitHub credential on fork builds.
curl --fail --silent --show-error --location --retry 3 \
  "https://api.github.com/repos/perplexityai/gazelle_py/pulls/$BUILDKITE_PULL_REQUEST" \
  --output "$work_dir/pull-request.json"
node - "$work_dir" <<'JS'
const fs = require('node:fs');
const path = process.argv[2];
const pr = JSON.parse(fs.readFileSync(`${path}/pull-request.json`, 'utf8'));
if (!/^[0-9a-f]{40}$/.test(pr.base.sha) || !/^[0-9a-f]{40}$/.test(pr.head.sha)) {
  throw new Error('Invalid pull request commit SHA');
}
fs.writeFileSync(`${path}/title`, `${pr.title}\n`);
fs.writeFileSync(`${path}/base`, pr.base.sha);
fs.writeFileSync(`${path}/head`, pr.head.sha);
JS
base=$(cat "$work_dir/base")
head=$(cat "$work_dir/head")
# Do not report a newer PR revision as validation of this build's commit.
if [[ "$head" != "$(git rev-parse HEAD)" ]]; then
  echo 'Pull request head changed; run a build for its current revision' >&2
  exit 1
fi
git fetch --no-tags origin "$base"
if [[ "$(git rev-parse --is-shallow-repository)" == true ]]; then
  git fetch --unshallow origin
fi

pnpm exec lefthook run commit-msg "$work_dir/title" --force

git rev-list --reverse "$base..$head" > "$work_dir/commits"
while IFS= read -r commit; do
  git show --no-patch --format=%B "$commit" > "$work_dir/message"
  pnpm exec lefthook run commit-msg "$work_dir/message" --force
done < "$work_dir/commits"
