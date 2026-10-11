#!/usr/bin/env bash
set -euo pipefail
source .buildkite/bazel.sh setup
bazel_command=bazel
python3 -B .buildkite/artifacts_test.py
root=$PWD
for platform in linux-amd64 darwin-arm64; do
  case "$platform" in
    linux-amd64) target_platform=@rules_rs//rs/platforms:x86_64-unknown-linux-gnu ;;
    darwin-arm64) target_platform=@rules_rs//rs/platforms:aarch64-apple-darwin ;;
  esac
  echo "--- Build $platform binaries"
  "$bazel_command" build --use_target_platform_for_tests "--platforms=$target_platform" //.buildkite:gazelle //py:py_test
  directory="$root/.buildkite-artifacts/smoke/$USE_BAZEL_VERSION/$platform"
  mkdir -p "$directory"
  for item in 'gazelle //.buildkite:gazelle' 'unit-test //py:py_test'; do
    read -r name target <<< "$item"
    binary=$("$bazel_command" cquery --use_target_platform_for_tests "--platforms=$target_platform" "$target" --output=starlark \
      '--starlark:expr=providers(target)["DefaultInfo"].files_to_run.executable.path')
    cp -L "$binary" "$directory/$name"
  done
  python3 .buildkite/artifacts.py pack "$directory" "$platform"
done
buildkite-agent artifact upload ".buildkite-artifacts/smoke/$USE_BAZEL_VERSION/**/*"
