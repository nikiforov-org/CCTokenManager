#!/bin/bash
# The Windows installer: Setup, the program beside this script, carrying the
# app for both architectures. No cgo, so it builds on any host. Run by
# builder/build.sh, which sets BIN, VERSION and DIST.
set -euo pipefail
cd "$(dirname "$0")/../.."

SETUP="builder/windows"

rm -f "$DIST/$BIN-"*-setup.exe

echo "==> Windows"
# Both architectures go into one amd64 installer, which Windows 11 on Arm runs
# under emulation. The icon, manifest and version info come from
# rsrc_windows_*.syso; the installer borrows the amd64 one.
PAYLOAD="$SETUP/payload"
mkdir -p "$PAYLOAD"
for arch in amd64 arm64; do
  GOOS=windows GOARCH=$arch CGO_ENABLED=0 \
    go build -trimpath -ldflags "-s -w -H windowsgui" -o "$PAYLOAD/$arch.exe" .
done
cp rsrc_windows_amd64.syso "$SETUP/"
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
  go build -trimpath -ldflags "-s -w -H windowsgui -X main.version=$VERSION" \
  -o "$DIST/$BIN-$VERSION-setup.exe" "./$SETUP"
rm -rf "$PAYLOAD" "$SETUP/rsrc_windows_amd64.syso"
