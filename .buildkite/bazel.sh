#!/usr/bin/env bash
set -euo pipefail

case "${1:-}" in
  test|examples|smoke) ;;
  *) echo "Usage: $0 {test|examples|smoke}" >&2; exit 1 ;;
esac
: "${USE_BAZEL_VERSION:?Set USE_BAZEL_VERSION}"

cache_dir=${CI_CACHE_DIR:-/tmp/gazelle-py-ci}
mkdir -p "$cache_dir/bin"
bazelisk="$cache_dir/bin/bazelisk"
case "$(uname -s)-$(uname -m)" in
  Linux-x86_64)
    bazelisk_platform=linux-amd64
    bazelisk_sha=5a408715e932c0250d28bd84555f12edbf70117de42f9181691c736eacc4a992
    ;;
  Darwin-arm64)
    bazelisk_platform=darwin-arm64
    bazelisk_sha=cee851f726789227d5561004e9904a52be45c3efb56f8b38b6993d6adbaa0409
    ;;
  *) echo 'Unsupported agent platform' >&2; exit 1 ;;
esac
if ! echo "$bazelisk_sha  $bazelisk" | shasum -a 256 --check --status 2>/dev/null; then
  curl --fail --silent --show-error --location --retry 3 \
    "https://github.com/bazelbuild/bazelisk/releases/download/v1.29.0/bazelisk-$bazelisk_platform" \
    --output "$bazelisk"
  echo "$bazelisk_sha  $bazelisk" | shasum -a 256 --check
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

if [[ "$1" == smoke ]]; then
  bazel test //py:py_test
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
