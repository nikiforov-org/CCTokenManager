#!/bin/bash
# The build of one architecture, inside its container: the app, then Setup with
# the app staged beside it for go:embed.
set -euo pipefail
PAYLOAD=builder/linux/payload
mkdir -p "$PAYLOAD"
go build -trimpath -ldflags "-s -w" -o "$PAYLOAD/$BIN" .
cp icon/app.png "$PAYLOAD/app.png"
arch=$([ "$ARCH" = amd64 ] && echo x64 || echo arm64)
go build -trimpath -ldflags "-s -w -X main.version=$VERSION" \
  -o "$DIST/$BIN-$VERSION-linux-$arch-setup" ./builder/linux
rm -rf "$PAYLOAD"
