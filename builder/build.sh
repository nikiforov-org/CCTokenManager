#!/bin/bash
# Builds the installers into dist: the macOS one, on macOS alone, with
# builder/macos, the Windows one with builder/windows, and the Linux ones, in
# Docker, with builder/linux. The version is set here and nowhere else.
set -euo pipefail
cd "$(dirname "$0")/.."

export BIN="CCTokenManager"
# Version is set by hand, never bumped automatically.
export VERSION="${VERSION:-0.0.1}"
export DIST="dist"
mkdir -p "$DIST"

if [[ "$(uname -s)" == Darwin* ]]; then
  bash builder/macos/build.sh
else
  echo "==> macOS installer skipped (not on macOS)"
fi
bash builder/windows/build.sh
bash builder/linux/build.sh

echo
echo "==> done"
ls -lh "$DIST"
