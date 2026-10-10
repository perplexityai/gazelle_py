#!/usr/bin/env bash
set -euo pipefail

case "${1:-}" in
  test|examples) ;;
  *) echo "Usage: $0 {test|examples}" >&2; exit 1 ;;
esac
: "${USE_BAZEL_VERSION:?Set USE_BAZEL_VERSION}"

# The oss queue runs Linux amd64. Cache misses must work on fresh agents.
cache_dir=${CI_CACHE_DIR:-/tmp/gazelle-py-ci}
mkdir -p "$cache_dir/bin"
bazelisk="$cache_dir/bin/bazelisk"
bazelisk_sha=5a408715e932c0250d28bd84555f12edbf70117de42f9181691c736eacc4a992
if ! echo "$bazelisk_sha  $bazelisk" | sha256sum --check --status 2>/dev/null; then
  curl --fail --silent --show-error --location --retry 3 \
    https://github.com/bazelbuild/bazelisk/releases/download/v1.29.0/bazelisk-linux-amd64 \
    --output "$bazelisk"
  echo "$bazelisk_sha  $bazelisk" | sha256sum --check
fi
chmod +x "$bazelisk"
export BAZELISK_HOME="$cache_dir/bazelisk"

bazel() {
  local command=$1
  shift
  "$bazelisk" "$command" \
    --repository_cache="$cache_dir/repository" \
    --disk_cache="$cache_dir/disk/$USE_BAZEL_VERSION" \
    "$@"
}

if [[ "$1" == test ]]; then
  bazel test //...
  exit
fi

for example in basic composite edge_cases file_mode project_mode naming_conventions; do
  echo "--- examples/$example"
  (
    cd "examples/$example"
    bazel test //...
    bazel run //:gazelle -- update -mode=diff
  )
done

# Analysis catches cross-platform toolchain regressions without linking.
cd examples/cross_compile
for triple in aarch64-apple-darwin x86_64-apple-darwin \
  aarch64-pc-windows-gnullvm x86_64-pc-windows-gnullvm; do
  echo "--- --platforms=$triple"
  bazel build --nobuild "--platforms=@rules_rs//rs/platforms:$triple" //:gazelle_bin
done
