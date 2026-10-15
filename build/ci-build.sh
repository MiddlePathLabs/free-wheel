#!/usr/bin/env bash
# Builds freewheel.exe the same way for CI builds and releases.
#
# Usage: build/ci-build.sh [velo-platform-tag]
#   With no tag, embeds the APKs from the latest velo-platform release.
# Needs: gh (GH_TOKEN set), go, a C compiler for cgo (Fyne).
set -euo pipefail
cd "$(dirname "$0")/.."

VELO_REPO="ril3y/velo-platform"
VELO_TAG="${1:-}"

# --- Embedded APKs ---
mkdir -p assets
rm -f assets/velolauncher.apk assets/serialbridge.apk
if [ -z "$VELO_TAG" ]; then
  VELO_TAG=$(gh release view -R "$VELO_REPO" --json tagName -q .tagName)
fi
echo "Embedding APKs from $VELO_REPO $VELO_TAG"
gh release download "$VELO_TAG" -R "$VELO_REPO" \
  -p velolauncher.apk -p serialbridge.apk -D assets

for apk in assets/velolauncher.apk assets/serialbridge.apk; do
  if [ ! -s "$apk" ]; then
    echo "::error::$apk missing or empty after download from $VELO_TAG"
    exit 1
  fi
  if [ "$(head -c 2 "$apk")" != "PK" ]; then
    echo "::error::$apk is not an APK (zip) file"
    exit 1
  fi
done
sha256sum assets/velolauncher.apk assets/serialbridge.apk

# Both APKs must be signed with the velo-platform platform key
# (keys/platform.x509.pem). The installer's update path replaces the
# sharedUserId=system SerialBridge; an APK signed with any other key would
# fail to install and leave the bike without sensors.
PLATFORM_CERT_SHA256="2d370c21f5dfd553d2a796314b70925fb38adeef90864c920bbbbb12887d3522"
if ! CGO_ENABLED=0 go run ./tools/apkcert "$PLATFORM_CERT_SHA256" \
    assets/velolauncher.apk assets/serialbridge.apk; then
  # Releases (STRICT_SIGNER=1) must never ship a mis-signed APK. CI builds only
  # warn, so PRs stay buildable while the latest velo-platform release predates
  # platform-signed launchers (v1.4.0 and earlier).
  if [ "${STRICT_SIGNER:-0}" = "1" ]; then
    echo "::error::APKs from $VELO_TAG are not signed with the platform key"
    exit 1
  fi
  echo "::warning::APKs from $VELO_TAG are not signed with the platform key (not a release build, continuing)"
fi

# --- Version info ---
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo dev)
VERSION=${VERSION#v}
HASH=$(git rev-parse --short=8 HEAD)
# Windows resource versions must be numeric a.b.c.d
WINVER=$(echo "$VERSION" | grep -oE '^[0-9]+\.[0-9]+\.[0-9]+' || true)
WINVER="${WINVER:-0.0.0}.0"
echo "Version $VERSION ($HASH), resource version $WINVER"

# --- Windows resources (icon, manifest, version). Optional. ---
if [ -f cmd/freewheel/winres/winres.json ]; then
  if go install github.com/tc-hib/go-winres@v0.3.3 && \
     (cd cmd/freewheel && go-winres make --arch amd64 \
        --product-version "$WINVER" --file-version "$WINVER"); then
    echo "Windows resources generated"
  else
    echo "::warning::go-winres failed; building without exe icon/version info"
    rm -f cmd/freewheel/*.syso
  fi
fi

# --- Build ---
export CGO_ENABLED=1 GOOS=windows GOARCH=amd64
go build -trimpath \
  -ldflags "-H windowsgui -X main.version=${VERSION} -X main.gitHash=${HASH}" \
  -o freewheel.exe ./cmd/freewheel
ls -l freewheel.exe

if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
  {
    echo "### freewheel.exe $VERSION ($HASH)"
    echo ""
    echo "Embedded APKs from \`$VELO_REPO\` **$VELO_TAG**:"
    echo '```'
    sha256sum assets/velolauncher.apk assets/serialbridge.apk freewheel.exe
    echo '```'
  } >> "$GITHUB_STEP_SUMMARY"
fi
if [ -n "${GITHUB_OUTPUT:-}" ]; then
  echo "velo_tag=$VELO_TAG" >> "$GITHUB_OUTPUT"
  echo "version=$VERSION" >> "$GITHUB_OUTPUT"
fi
