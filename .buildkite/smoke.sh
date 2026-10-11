#!/usr/bin/env bash
set -euo pipefail
: "${USE_BAZEL_VERSION:?}"
: "${SMOKE_PRODUCER_STEP:?}"
case "$(uname -s)-$(uname -m)" in
  Linux-x86_64) platform=linux-amd64 ;;
  Darwin-arm64) platform=darwin-arm64 ;;
  *) echo 'Unsupported smoke platform' >&2; exit 1 ;;
esac
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
artifact=".buildkite-artifacts/smoke/$USE_BAZEL_VERSION/$platform"
buildkite-agent artifact download "$artifact/*" "$work" \
  --build "${BUILDKITE_BUILD_ID:?}" --step "$SMOKE_PRODUCER_STEP"
directory="$work/$artifact"
python3 .buildkite/artifacts.py verify "$directory" "$platform"
export TEST_TMPDIR="$work/tests"
mkdir -p "$TEST_TMPDIR"
"$directory/unit-test" -test.v
mkdir -p "$work/fixture"
printf 'module(name = "smoke")\n' > "$work/fixture/MODULE.bazel"
printf 'import os\n' > "$work/fixture/main.py"
"$directory/gazelle" -repo_root="$work/fixture" "$work/fixture"
grep -q 'py_library(' "$work/fixture/BUILD.bazel"
"$directory/gazelle" -repo_root="$work/fixture" -mode=diff "$work/fixture"
