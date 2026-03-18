#!/usr/bin/env bash
# build.sh — cross-compile DNSReconciler binaries with embedded version and Windows icon.
#
# Usage:
#   ./scripts/build.sh              # auto-generates version from UTC timestamp
#   ./scripts/build.sh 2026.03.18.1700  # explicit version override
#
set -euo pipefail

export PATH="${PATH}:$(go env GOPATH)/bin"

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SRC_DIR="${REPO_ROOT}/src"
ARTIFACTS_DIR="${REPO_ROOT}/Artifacts"
CMD_DIR="${SRC_DIR}/cmd/dnsreconciler"
ICON_PATH="${REPO_ROOT}/resources/icons/dns-00001.ico"

# Version: use argument if provided, otherwise generate from UTC timestamp.
VERSION="${1:-$(date -u +"%Y.%m.%d.%H%M")}"
LDFLAGS="-s -w -X github.com/gracesolutions/dns-automatic-updater/internal/app.Version=${VERSION}"

echo "=== DNSReconciler Build ==="
echo "  Version : ${VERSION}"
echo "  Output  : ${ARTIFACTS_DIR}"
echo ""

mkdir -p "${ARTIFACTS_DIR}"

# ---- Generate Windows resource (.syso) with embedded icon ----
echo "[1/7] Generating Windows resource (icon + version info)..."
GOVERSIONINFO="$(go env GOPATH)/bin/goversioninfo"
if [ ! -x "${GOVERSIONINFO}" ]; then
    echo "  Installing goversioninfo..."
    go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest
fi

# Update versioninfo.json with current version strings
VINFO="${CMD_DIR}/versioninfo.json"
if [ -f "${VINFO}" ]; then
    # Parse version components: yyyy.mm.dd.hhmm → Major=yyyy Minor=mm Patch=dd Build=hhmm
    IFS='.' read -r V_MAJOR V_MINOR V_PATCH V_BUILD <<< "${VERSION}"
    # Strip leading zeros for Python integer literals
    V_MAJOR=$((10#${V_MAJOR})); V_MINOR=$((10#${V_MINOR})); V_PATCH=$((10#${V_PATCH})); V_BUILD=$((10#${V_BUILD}))
    if command -v python3 &>/dev/null; then
        python3 -c "
import json
with open('${VINFO}') as f:
    vi = json.load(f)
vi['FixedFileInfo']['FileVersion'] = {'Major': ${V_MAJOR}, 'Minor': ${V_MINOR}, 'Patch': ${V_PATCH}, 'Build': ${V_BUILD}}
vi['FixedFileInfo']['ProductVersion'] = {'Major': ${V_MAJOR}, 'Minor': ${V_MINOR}, 'Patch': ${V_PATCH}, 'Build': ${V_BUILD}}
vi['StringFileInfo']['FileVersion'] = '${VERSION}'
vi['StringFileInfo']['ProductVersion'] = '${VERSION}'
with open('${VINFO}', 'w') as f:
    json.dump(vi, f, indent=4)
    f.write('\n')
"
        echo "  ✓ versioninfo.json updated to ${VERSION}"
    else
        echo "  (python3 not available — skipping versioninfo.json update)"
    fi
fi

(cd "${CMD_DIR}" && "${GOVERSIONINFO}" -64 -icon="${ICON_PATH}" -o resource_windows.syso)
echo "  ✓ resource_windows.syso generated"

# ---- Build targets ----
TARGETS=(
    "linux:amd64"
    "linux:arm64"
    "darwin:amd64"
    "darwin:arm64"
    "windows:amd64"
    "windows:arm64"
)

TOTAL=$((${#TARGETS[@]} + 1))
STEP=2
for target in "${TARGETS[@]}"; do
    IFS=':' read -r GOOS GOARCH <<< "${target}"
    SUFFIX=""
    [ "${GOOS}" = "windows" ] && SUFFIX=".exe"
    OUT="${ARTIFACTS_DIR}/dnsreconciler-${GOOS}-${GOARCH}${SUFFIX}"

    # Regenerate .syso for the correct architecture when switching Windows targets
    if [ "${GOOS}" = "windows" ]; then
        ARCH_FLAG="-64"
        [ "${GOARCH}" = "arm64" ] && ARCH_FLAG="-arm"
        (cd "${CMD_DIR}" && "${GOVERSIONINFO}" ${ARCH_FLAG} -icon="${ICON_PATH}" -o resource_windows.syso 2>/dev/null)
    fi

    echo "[${STEP}/${TOTAL}] Building ${GOOS}/${GOARCH}..."
    (cd "${SRC_DIR}" && CGO_ENABLED=0 GOOS="${GOOS}" GOARCH="${GOARCH}" \
        go build -ldflags="${LDFLAGS}" -o "${OUT}" ./cmd/dnsreconciler)
    echo "  ✓ ${OUT##*/}"
    STEP=$((STEP + 1))
done

echo ""
echo "=== Build complete: version ${VERSION} ==="

