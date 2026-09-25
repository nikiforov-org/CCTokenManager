#!/bin/bash
# Builds the installers here with builder/build.sh and publishes dist as the
# GitHub release vVERSION, tagged at the commit they were built from. A release
# that exists already is left alone: a new build needs a new version.
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${VERSION:-$(sed -n 's/^export VERSION="\${VERSION:-\(.*\)}"$/\1/p' builder/build.sh)}"
TAG="v$VERSION"

# The release is tagged at the commit on GitHub the build came from, so what
# is built has to be that commit and nothing besides.
if [ -n "$(git status --porcelain)" ]; then
  echo "Not released: the working tree has changes not committed." >&2
  exit 1
fi
git fetch --quiet origin
HEAD="$(git rev-parse HEAD)"
if ! git branch -r --contains "$HEAD" | grep -q .; then
  echo "Not released: $(git rev-parse --short HEAD) is not pushed to GitHub." >&2
  exit 1
fi
if gh release view "$TAG" >/dev/null 2>&1; then
  echo "Not released: $TAG is out already. Set a new VERSION in builder/build.sh." >&2
  exit 1
fi
if [[ "$(uname -s)" != Darwin* ]]; then
  echo "Not released: the macOS installer is built on macOS alone." >&2
  exit 1
fi

# dist holds this build and nothing left from another.
rm -rf dist
VERSION="$VERSION" bash builder/build.sh

echo
echo "==> release $TAG"
gh release create "$TAG" --target "$HEAD" --title "CC Token Manager $VERSION" --notes "" dist/*
