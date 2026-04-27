#!/usr/bin/env bash
#
# release_prep.sh — produce the source archive and BCR-ready release notes for
# a `vX.Y.Z` tag. Mirrors the rules_rs / aspect-build pattern.
#
# Usage: release_prep.sh <tag>           (e.g. v0.1.0)
#        TAG is also picked up from $GITHUB_REF_NAME when no arg is given.
#
# Stdout: release notes (markdown) ready to feed to softprops/action-gh-release.
# Side effect: writes ${MODULE}-${TAG}.tar.gz into the working directory.

set -o errexit -o nounset -o pipefail

TAG="${1:-${GITHUB_REF_NAME:-}}"
if [[ -z "${TAG}" ]]; then
    echo "release_prep.sh: tag is required (got empty)" >&2
    exit 1
fi

# source.template.json uses {VERSION} (no leading "v") for strip_prefix and
# {TAG} (with "v") in the URL — mirror both. The tarball *file* is named with
# {TAG}; the directory *inside* uses {VERSION}.
VERSION="${TAG#v}"
MODULE=gazelle_py
PREFIX="${MODULE}-${VERSION}"
ARCHIVE="${MODULE}-${TAG}.tar.gz"

git archive --format=tar --prefix="${PREFIX}/" "${TAG}" | gzip -9 > "${ARCHIVE}"

# Subresource Integrity (SRI): "sha256-<base64(sha256(archive))>". This is what
# bazel_dep's source.json wants, and what publish-to-bcr will fill in for us —
# we compute it here too so the release notes stay self-contained.
SHA256_HEX=$(shasum -a 256 "${ARCHIVE}" | awk '{print $1}')
SHA256_B64=$(printf '%s' "${SHA256_HEX}" | xxd -r -p | base64)
INTEGRITY="sha256-${SHA256_B64}"

cat <<EOF
## Using Bzlmod with Bazel 7+

Add to your \`MODULE.bazel\`:

\`\`\`starlark
bazel_dep(name = "${MODULE}", version = "${VERSION}")
\`\`\`

## Using a non-registry override

\`\`\`starlark
bazel_dep(name = "${MODULE}", version = "${VERSION}")

archive_override(
    module_name = "${MODULE}",
    integrity = "${INTEGRITY}",
    strip_prefix = "${PREFIX}",
    urls = ["https://github.com/${GITHUB_REPOSITORY:-hermeticbuild/${MODULE}}/releases/download/${TAG}/${ARCHIVE}"],
)
\`\`\`
EOF
