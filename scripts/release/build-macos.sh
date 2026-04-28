#!/bin/bash
set -euo pipefail
ARCH="${1:?usage: build-macos.sh arm64|amd64 VERSION [--skip-notarize]}"
VERSION="${2:?usage: build-macos.sh ARCH VERSION [--skip-notarize]}"
SKIP_NOTARIZE="${3:-}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
TEMPLATES_DIR="$REPO_ROOT/templates/release"
ENTITLEMENTS_PATH="$SCRIPT_DIR/entitlements.plist"

# Signing identity and notarization profile are env-driven so the same build
# script works on any machine or CI runner. The signing identity env var name
# matches bldr's signing hook so a single export covers both layers.
SIGNING_ID="${BLDR_MACOS_SIGN_IDENTITY:-}"
if [ -z "$SIGNING_ID" ]; then
  echo "ERROR: BLDR_MACOS_SIGN_IDENTITY is not set." >&2
  echo "Example: export BLDR_MACOS_SIGN_IDENTITY='Developer ID Application: Your Team (TEAMID)'" >&2
  exit 1
fi

# Notarization keychain profile. Bootstrap once with:
#   xcrun notarytool store-credentials "$BLDR_MACOS_NOTARIZE_PROFILE" --key ~/path/to/AuthKey_KEYID.p8 --key-id KEYID --issuer ISSUERID
NOTARIZE_PROFILE="${BLDR_MACOS_NOTARIZE_PROFILE:-spacewave-notarize}"

# Bundle id for the SMJobBless privileged helper tool. Referenced by:
#   - Info.plist SMPrivilegedExecutables key
#   - Launchd.plist Label (+ filename under LaunchServices/)
#   - codesign --identifier + designated requirement
#   - Swift UpdateManager.helperLabel + helperInstalledPath
# Must match the Swift side exactly or SMJobBless fails with
# kSMErrorAuthorizationFailure at runtime.
HELPER_LABEL="us.aperture.spacewave.helper"

BINARY="$REPO_ROOT/.tmp/dist/darwin-${ARCH}/spacewave"
ICON="$REPO_ROOT/.tmp/icons/icon.icns"
HELPER="$REPO_ROOT/dist/helper/darwin-${ARCH}/spacewave-helper"
OUT="$REPO_ROOT/dist/installers"
APP="$REPO_ROOT/.tmp/Spacewave.app"
APP_ZIP="$REPO_ROOT/.tmp/Spacewave-${ARCH}.zip"

# SMJobBless requires both Info.plist files (main app + privileged tool) to
# reference the signing team ID in their SMPrivilegedExecutables /
# SMAuthorizedClients code requirements. Extract the team ID from the
# parenthesized tail of the signing identity, e.g. "Developer ID
# Application: Aperture Robotics LLC (ABCDE12345)" -> "ABCDE12345".
TEAM_ID="$(printf '%s' "$SIGNING_ID" | sed -n 's/.*(\([A-Z0-9]\{10\}\)).*/\1/p')"
if [ -z "$TEAM_ID" ]; then
  echo "ERROR: could not extract team id from BLDR_MACOS_SIGN_IDENTITY: $SIGNING_ID" >&2
  exit 1
fi

mkdir -p "$OUT"

# Verify build artifacts exist.
if [ ! -f "$BINARY" ]; then
  echo "ERROR: missing entrypoint binary: $BINARY" >&2
  exit 1
fi

# --- 0. Build the macOS Swift helpers (loading window + privileged swap) ---
# The loading helper (=SpacewaveHelper=) was previously built by
# =scripts/build-helper.sh=; keep that path for =$HELPER= but additionally
# build the privileged tool (=SpacewaveHelperPrivileged=) here with plist
# sections embedded at link time. The privileged tool's Info.plist needs
# {{VERSION}} and {{TEAM_ID}} substituted before linking so SMJobBless can
# match the signature requirement at install time. The rendered plists are
# arch-independent so they live in a shared path regardless of $ARCH.
PRIV_PLIST_DIR="$REPO_ROOT/.tmp/macos-helper-plists"
mkdir -p "$PRIV_PLIST_DIR"
sed -e "s/{{VERSION}}/$VERSION/g" -e "s/{{TEAM_ID}}/$TEAM_ID/g" \
  "$TEMPLATES_DIR/macos-helper/Info.plist" > "$PRIV_PLIST_DIR/Info.plist"
sed -e "s/{{VERSION}}/$VERSION/g" -e "s/{{TEAM_ID}}/$TEAM_ID/g" \
  "$TEMPLATES_DIR/macos-helper/Launchd.plist" > "$PRIV_PLIST_DIR/Launchd.plist"

case "$ARCH" in
  amd64) SWIFT_ARCH="x86_64" ;;
  arm64) SWIFT_ARCH="arm64" ;;
  *)
    echo "ERROR: unsupported ARCH for macOS helper build: $ARCH" >&2
    exit 1
    ;;
esac

(
  cd "$REPO_ROOT/desktop/macos" && \
  swift build -c release --arch "$SWIFT_ARCH" \
    --product SpacewaveHelperPrivileged \
    -Xlinker -sectcreate \
    -Xlinker __TEXT \
    -Xlinker __info_plist \
    -Xlinker "$PRIV_PLIST_DIR/Info.plist" \
    -Xlinker -sectcreate \
    -Xlinker __TEXT \
    -Xlinker __launchd_plist \
    -Xlinker "$PRIV_PLIST_DIR/Launchd.plist"
)
PRIV_BIN="$REPO_ROOT/desktop/macos/.build/${SWIFT_ARCH}-apple-macosx/release/SpacewaveHelperPrivileged"
if [ ! -f "$PRIV_BIN" ]; then
  echo "ERROR: privileged helper build did not produce $PRIV_BIN" >&2
  exit 1
fi

# --- 1. Assemble .app bundle ---
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS"
mkdir -p "$APP/Contents/Resources"
mkdir -p "$APP/Contents/Library/LaunchServices"

# Info.plist from template. SMPrivilegedExecutables carries the team id so
# the blessed helper tool's signature can be verified against this main
# app's entry without any runtime string building.
sed -e "s/{{VERSION}}/$VERSION/g" -e "s/{{TEAM_ID}}/$TEAM_ID/g" \
  "$TEMPLATES_DIR/Info.plist" > "$APP/Contents/Info.plist"
printf 'APPL????' > "$APP/Contents/PkgInfo"

# Embed the privileged helper tool. SMJobBless copies the file named after
# its label from =Contents/Library/LaunchServices/= into
# =/Library/PrivilegedHelperTools/=, so the filename here must match the
# label exactly.
cp "$PRIV_BIN" "$APP/Contents/Library/LaunchServices/$HELPER_LABEL"
chmod +x "$APP/Contents/Library/LaunchServices/$HELPER_LABEL"

# Go entrypoint binary as the main executable.
# CFBundleExecutable in Info.plist is "Spacewave".
cp "$BINARY" "$APP/Contents/MacOS/Spacewave"
chmod +x "$APP/Contents/MacOS/Spacewave"

# Icon.
if [ -f "$ICON" ]; then
  cp "$ICON" "$APP/Contents/Resources/app.icns"
fi

# Native helper binary for loading screen and self-update.
if [ -f "$HELPER" ]; then
  cp "$HELPER" "$APP/Contents/MacOS/spacewave-helper"
  chmod +x "$APP/Contents/MacOS/spacewave-helper"
fi

# --- 2. Code sign ---
# Sign inner binaries first, then the whole .app.
if [ -f "$APP/Contents/MacOS/spacewave-helper" ]; then
  codesign --force \
    --sign "$SIGNING_ID" \
    --options runtime \
    "$APP/Contents/MacOS/spacewave-helper"
fi

# SMJobBless validates the privileged tool by the exact code-requirement
# string embedded in the main app's SMPrivilegedExecutables entry, so sign
# the tool with a literal =--requirements= matching that entry. The team
# id must match =$TEAM_ID= extracted above.
PRIV_REQ="designated => anchor apple generic and identifier \"$HELPER_LABEL\" and certificate leaf[subject.OU] = \"$TEAM_ID\""
codesign --force \
  --sign "$SIGNING_ID" \
  --options runtime \
  --identifier "$HELPER_LABEL" \
  --requirements "=$PRIV_REQ" \
  "$APP/Contents/Library/LaunchServices/$HELPER_LABEL"

# Sign the outer bundle WITHOUT --deep: --deep retraverses nested code and
# re-signs it using codesign's default identifier inference, wiping the
# explicit --identifier we gave the privileged helper. We have already
# signed every inner binary above (spacewave-helper + the SMJobBless tool),
# so the bundle sign only needs to cover the bundle itself. The
# --verify --deep --strict check below still validates the nested
# signatures remain intact.
codesign --force \
  --sign "$SIGNING_ID" \
  --options runtime \
  --entitlements "$ENTITLEMENTS_PATH" \
  "$APP"

# Verify signature.
codesign --verify --deep --strict "$APP"
# Double-check the privileged tool's bundle identifier survived signing.
# If SMJobBless sees the wrong identifier it refuses at runtime with the
# unhelpful =kSMErrorAuthorizationFailure=, so catch it at build time.
# Capture the output first: piping directly into =grep -q= under
# =set -o pipefail= exits the pipeline with 141 (SIGPIPE on codesign) as
# soon as grep finds its match, which then trips the error branch even
# when the identifier is present.
PRIV_CODESIGN_OUT="$(codesign --display --verbose=4 \
  "$APP/Contents/Library/LaunchServices/$HELPER_LABEL" 2>&1)"
if ! printf '%s\n' "$PRIV_CODESIGN_OUT" | grep -q "^Identifier=$HELPER_LABEL\$"; then
  echo "ERROR: privileged helper bundle identifier did not survive signing" >&2
  echo "$PRIV_CODESIGN_OUT" >&2
  exit 1
fi
echo "Signature verified."

DMG="$OUT/spacewave-macos-${ARCH}.dmg"

# build_installer_dmg emits a branded DMG by stamping a pre-captured Finder
# layout (.DS_Store + background.png) onto a writable image, then compressing
# to UDZO. The layout was set up once interactively in Finder and captured
# into templates/dmg/ (see the DMG template notes). To re-capture, edit the
# background in GIMP (saving to templates/dmg/background.png), then redo the
# Finder dance: create an RW DMG, drop .app + .background/background.png +
# Applications symlink, open in Finder, position icons + set window size +
# pick the background, close the window, copy out .DS_Store and background.
#
# Prerequisites: volume name "Spacewave", .app named "Spacewave.app",
# Applications symlink at the volume root, background at
# .background/background.png. Any rename breaks the captured positions.
build_installer_dmg() {
  local STAGE=".tmp/dmg-stage-${ARCH}"
  local RWDMG=".tmp/dmg-rw-${ARCH}.dmg"
  local MOUNT="/Volumes/Spacewave"

  rm -f "$DMG" "$RWDMG"
  rm -rf "$STAGE"
  mkdir -p "$STAGE/.background"

  # Stage the payload. The .app carries over the stapled ticket (step 3).
  cp -R "$APP" "$STAGE/Spacewave.app"
  cp "$TEMPLATES_DIR/dmg/background.png" "$STAGE/.background/background.png"
  cp "$TEMPLATES_DIR/dmg/DS_Store" "$STAGE/.DS_Store"
  ln -s /Applications "$STAGE/Applications"

  # Build a writable image sized from the staged payload so arch-specific app
  # size differences do not overflow a fixed image. Add 96 MiB for HFS+
  # overhead, Finder metadata, and compression staging slack.
  local STAGE_KB
  local DMG_SIZE_MB
  STAGE_KB="$(du -sk "$STAGE" | awk '{print $1}')"
  DMG_SIZE_MB="$(( (STAGE_KB + 98304 + 1023) / 1024 ))"
  hdiutil create -quiet -size "${DMG_SIZE_MB}m" -fs HFS+ -volname "Spacewave" \
    -srcfolder "$STAGE" -format UDRW "$RWDMG"

  # Compress to final read-only UDZO. hdiutil preserves .DS_Store and the
  # hidden .background/ directory across the convert.
  hdiutil convert -quiet "$RWDMG" -format UDZO -o "$DMG"

  rm -f "$RWDMG"
  rm -rf "$STAGE"

  # Double-check the mount is not lingering from a prior run.
  if [ -d "$MOUNT" ]; then
    hdiutil detach -quiet -force "$MOUNT" 2>/dev/null || true
  fi
}

if [ "$SKIP_NOTARIZE" = "--skip-notarize" ]; then
  # Unticketed DMG path for iteration / local dev only.
  build_installer_dmg
  codesign --sign "$SIGNING_ID" "$DMG"
  echo "Skipping notarization (--skip-notarize)."
  echo "Built: $DMG"
  exit 0
fi

# --- 3. Notarize + staple the inner .app ---
# Why both: the DMG ticket only covers the DMG itself. A user who copies the
# extracted .app onto a USB stick or another machine still needs a ticket
# attached to the .app to pass Gatekeeper offline. Staple the .app before
# packing it into the DMG so the extracted bundle carries its own ticket.
rm -f "$APP_ZIP"
ditto -c -k --keepParent "$APP" "$APP_ZIP"
xcrun notarytool submit "$APP_ZIP" \
  --keychain-profile "$NOTARIZE_PROFILE" --wait
xcrun stapler staple "$APP"
rm -f "$APP_ZIP"

# --- 4. Create DMG from the stapled .app ---
build_installer_dmg
codesign --sign "$SIGNING_ID" "$DMG"

# --- 5. Notarize + staple the DMG ---
# Apple cross-references the already-notarized .app bytes and returns the
# ticket quickly; staple pins it to the DMG so offline verification works.
xcrun notarytool submit "$DMG" \
  --keychain-profile "$NOTARIZE_PROFILE" --wait
xcrun stapler staple "$DMG"

echo "Built and notarized: $DMG"
