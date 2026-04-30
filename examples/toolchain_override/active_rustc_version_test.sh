#!/usr/bin/env bash
# --- begin runfiles.bash initialization v3 ---
set -uo pipefail; f=bazel_tools/tools/bash/runfiles/runfiles.bash
source "${RUNFILES_DIR:-/dev/null}/$f" 2>/dev/null || \
  source "$(grep -sm1 "^$f " "${RUNFILES_MANIFEST_FILE:-/dev/null}" | cut -f2- -d' ')" 2>/dev/null || \
  source "$0.runfiles/$f" 2>/dev/null || \
  source "$(grep -sm1 "^$f " "$0.runfiles_manifest" | cut -f2- -d' ')" 2>/dev/null || \
  { echo>&2 "ERROR: cannot find $f"; exit 1; }
# --- end runfiles.bash initialization v3 ---
set -e

expected="${1:?expected version arg missing}"
version_file="$(rlocation "${VERSION_FILE}")"
if [[ -z "${version_file}" || ! -f "${version_file}" ]]; then
  echo "FAIL: could not locate version file via rlocation '${VERSION_FILE}'" >&2
  exit 1
fi

actual="$(cat "${version_file}")"
if ! grep -qF "rustc ${expected}" "${version_file}"; then
  echo "FAIL: expected rustc ${expected}, got: ${actual}" >&2
  echo "  This usually means the consumer's toolchain registration is no" >&2
  echo "  longer winning over gazelle_py's. Check MODULE.bazel ordering." >&2
  exit 1
fi

echo "OK: rustc ${expected} active (${actual})"
