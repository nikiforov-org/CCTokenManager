#!/bin/bash
# The macOS installer: the disk image the app is dragged out of into
# Applications. The app bundle is made in scratch, and dist gets only the
# image. Run by builder/build.sh, which sets BIN, VERSION and DIST.
set -euo pipefail
cd "$(dirname "$0")/../.."

APP="CC Token Manager"
ID="org.nikiforov.CCTokenManager"
# The writable image is scratch too, and made where scratch goes: not every
# volume takes one (an exFAT drive refuses it as busy).
WORK="$(mktemp -d)"
BUNDLE="$WORK/$APP.app"

rm -f "$DIST/$BIN-"*.dmg

echo "==> app bundle"
mkdir -p "$BUNDLE/Contents/MacOS" "$BUNDLE/Contents/Resources"

go build -trimpath -ldflags "-s -w" -o "$BUNDLE/Contents/MacOS/$BIN" .
cp icon/AppIcon.icns "$BUNDLE/Contents/Resources/"

cat > "$BUNDLE/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key><string>$APP</string>
  <key>CFBundleIdentifier</key><string>$ID</string>
  <key>CFBundleExecutable</key><string>$BIN</string>
  <key>CFBundleIconFile</key><string>AppIcon</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleVersion</key><string>$VERSION</string>
  <key>CFBundleShortVersionString</key><string>$VERSION</string>
  <key>LSMinimumSystemVersion</key><string>11.0</string>
  <key>LSApplicationCategoryType</key><string>public.app-category.developer-tools</string>
  <key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
PLIST

# Signed ad hoc as a whole bundle: the Go linker signs the binary at most.
codesign --force --sign - "$BUNDLE"

echo "==> dmg"
# Finder decides icon order alphabetically unless the image carries a saved
# layout, which puts Applications before the app. Build a writable image,
# place the icons, then compress it.
STAGE="$(mktemp -d)"
cp -R "$BUNDLE" "$STAGE/"
ln -s /Applications "$STAGE/Applications"

# A stale volume of the same name makes the new one mount as "NAME 1", and the
# layout would then be applied to the wrong disk.
for v in "/Volumes/$APP"*; do
  [ -d "$v" ] && hdiutil detach -quiet -force "$v" 2>/dev/null
done

RW="$WORK/rw.dmg"
# No -size: hdiutil works out what the contents need.
hdiutil create -quiet -srcfolder "$STAGE" -volname "$APP" \
  -fs HFS+ -format UDRW "$RW"
rm -rf "$STAGE"

MOUNT="/Volumes/$APP"
hdiutil attach "$RW" -nobrowse -noautoopen -mountpoint "$MOUNT" >/dev/null

# Sizes are stated, not read: an icon size not written into the image is the
# viewer's own. A position is an icon's centre, measured from the top of the
# content, so the title bar comes off the window's height.
ICON=96
WIN_W=520
WIN_H=360
TITLEBAR=28                       # inside the bounds, outside the content
LABEL=22                          # the name under the icon
CONTENT_H=$((WIN_H - TITLEBAR))
ICON_Y=$(( (CONTENT_H - ICON - LABEL) / 2 + ICON / 2 ))
LEFT_X=$((WIN_W / 4))
RIGHT_X=$((WIN_W * 3 / 4))

osascript >/dev/null <<APPLESCRIPT || echo "    Finder did not lay the window out"
tell application "Finder"
  tell disk "$APP"
    open
    set current view of container window to icon view
    set toolbar visible of container window to false
    set statusbar visible of container window to false
    set theViewOptions to the icon view options of container window
    set arrangement of theViewOptions to not arranged
    set icon size of theViewOptions to $ICON
    set {left_, top_, right_, bottom_} to bounds of container window
    set bounds of container window to ¬
      {left_, top_, left_ + $WIN_W, top_ + $WIN_H}
    -- the app first, the folder it goes into second: arranging by name would
    -- put them the other way round
    set position of item "$APP.app" of container window to {$LEFT_X, $ICON_Y}
    set position of item "Applications" of container window to {$RIGHT_X, $ICON_Y}
    update without registering applications
    delay 2
    close
  end tell
end tell
APPLESCRIPT

sync
hdiutil detach -quiet "$MOUNT"
hdiutil convert -quiet "$RW" -format UDZO -o "$DIST/$BIN-$VERSION.dmg"
rm -rf "$WORK"
