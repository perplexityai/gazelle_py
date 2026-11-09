#!/usr/bin/env bash
set -euo pipefail
work_dir=$(mktemp -d)
trap 'rm -rf "$work_dir"' EXIT
npm install --global --prefix "$work_dir/pnpm" "$(node -p 'require("./package.json").packageManager')"
export PATH="$work_dir/pnpm/bin:$PATH"
pnpm install --frozen-lockfile
work_dir=$(mktemp -d)
trap 'rm -rf "$work_dir"' EXIT
if [[ "$GITHUB_EVENT_NAME" == pull_request ]]; then
  python3 - "$work_dir" <<'EVENT'
import json, os, pathlib
root = pathlib.Path(__import__('sys').argv[1])
pr = json.load(open(os.environ['GITHUB_EVENT_PATH']))['pull_request']
for key in ('base', 'head'):
    (root / key).write_text(pr[key]['sha'])
(root / 'title').write_text(pr['title'] + '\n')
EVENT
  base=$(cat "$work_dir/base")
  head=$(cat "$work_dir/head")
  export PR_TITLE="$(cat "$work_dir/title")"
else
  base=$(git rev-parse HEAD^)
  head=$(git rev-parse HEAD)
fi
[[ "$GITHUB_EVENT_NAME" == pull_request ]] || exit 0
pnpm exec lefthook run commit-msg "$work_dir/title" --force
git rev-list --reverse "$base..$head" > "$work_dir/commits"
while IFS= read -r commit; do
  git show --no-patch --format=%B "$commit" > "$work_dir/message"
  pnpm exec lefthook run commit-msg "$work_dir/message" --force
done < "$work_dir/commits"
