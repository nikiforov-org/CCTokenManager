#!/bin/bash
# The Linux installers: Setup for x64 and for arm64, each carrying the app built
# for it. cgo against GTK needs Linux, so each is built in a container of its
# architecture. Run by builder/build.sh, which sets BIN, VERSION and DIST.
set -euo pipefail
cd "$(dirname "$0")/../.."

rm -f "$DIST/$BIN-"*-linux-*-setup
GO_VERSION="$(go env GOVERSION | sed 's/^go//')"
for arch in amd64 arm64; do
  echo "==> Linux $arch"
  image="cc-token-manager-linux-builder:$arch-$GO_VERSION"
  docker build -q --platform "linux/$arch" --build-arg GO_VERSION="$GO_VERSION" \
    -t "$image" builder/linux >/dev/null
  docker run --rm --platform "linux/$arch" -v "$PWD":/src -w /src \
    -v "cc-token-manager-gocache-$arch:/root/.cache" -e BIN -e VERSION -e DIST -e ARCH="$arch" \
    "$image" bash builder/linux/inside.sh
done
