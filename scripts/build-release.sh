#!/usr/bin/env bash
set -euo pipefail

VERSION=""
UPLOAD_URL=""
API_KEY=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --upload)
      UPLOAD_URL="$2"
      shift 2
      ;;
    --api-key)
      API_KEY="$2"
      shift 2
      ;;
    -*)
      echo "Unknown option: $1"
      exit 1
      ;;
    *)
      if [ -z "$VERSION" ]; then
        VERSION="$1"
      else
        echo "Unexpected argument: $1"
        exit 1
      fi
      shift
      ;;
  esac
done

if [ -z "$VERSION" ]; then
  VERSION="v0.1.0"
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="${ROOT_DIR}/dist"

echo "Building release artifacts for version: ${VERSION}"
rm -rf "${DIST_DIR}"
mkdir -p "${DIST_DIR}"

PLATFORMS=(
  "linux/amd64"
  "linux/arm64"
  "darwin/amd64"
  "darwin/arm64"
  "windows/amd64"
)

LDFLAGS="-s -w -X 'main.Version=${VERSION}'"

for PLATFORM in "${PLATFORMS[@]}"; do
  GOOS="${PLATFORM%/*}"
  GOARCH="${PLATFORM#*/}"
  EXT=""
  if [ "$GOOS" = "windows" ]; then
    EXT=".exe"
  fi

  ARCHIVE_NAME="statusline_${VERSION}_${GOOS}_${GOARCH}"
  STAGE_DIR="${DIST_DIR}/${ARCHIVE_NAME}"
  mkdir -p "${STAGE_DIR}"

  echo "Compiling ${GOOS}/${GOARCH} with version ${VERSION}..."
  CGO_ENABLED=0 GOOS="${GOOS}" GOARCH="${GOARCH}" go build -ldflags="${LDFLAGS}" -o "${STAGE_DIR}/statusline${EXT}" "${ROOT_DIR}/cmd/statusline"
  CGO_ENABLED=0 GOOS="${GOOS}" GOARCH="${GOARCH}" go build -ldflags="${LDFLAGS}" -o "${STAGE_DIR}/statusline-server${EXT}" "${ROOT_DIR}/cmd/statusline-server"
  [ -f "${ROOT_DIR}/client.yaml" ] && cp "${ROOT_DIR}/client.yaml" "${STAGE_DIR}/"
  [ -f "${ROOT_DIR}/server.yaml" ] && cp "${ROOT_DIR}/server.yaml" "${STAGE_DIR}/"
  if [ -f "${ROOT_DIR}/AGENTS.md" ]; then
    cp "${ROOT_DIR}/AGENTS.md" "${STAGE_DIR}/"
  fi

  cd "${DIST_DIR}"
  if [ "$GOOS" = "windows" ]; then
    if command -v zip >/dev/null 2>&1; then
      zip -rq "${ARCHIVE_NAME}.zip" "${ARCHIVE_NAME}"
    else
      python3 -m zipfile -c "${ARCHIVE_NAME}.zip" "${ARCHIVE_NAME}"
    fi
  else
    tar -czf "${ARCHIVE_NAME}.tar.gz" "${ARCHIVE_NAME}"
  fi
  rm -rf "${STAGE_DIR}"
  cd "${ROOT_DIR}"
done

echo "Generating sha256 checksums..."
cd "${DIST_DIR}"
sha256sum * > checksums.txt
cd "${ROOT_DIR}"

echo "Artifacts built successfully in ${DIST_DIR}:"
ls -lh "${DIST_DIR}"

if [ -n "$UPLOAD_URL" ]; then
  echo "Uploading built artifacts to ${UPLOAD_URL}/api/v1/upgrade/upload..."
  CURL_ARGS=(-s -S -X POST)
  if [ -n "$API_KEY" ]; then
    CURL_ARGS+=(-H "X-API-Key: ${API_KEY}")
  fi
  for f in "${DIST_DIR}"/*; do
    if [ -f "$f" ]; then
      CURL_ARGS+=(-F "files=@${f}")
    fi
  done
  UPLOAD_ENDPOINT="${UPLOAD_URL%/}/api/v1/upgrade/upload"
  curl "${CURL_ARGS[@]}" "${UPLOAD_ENDPOINT}"
  echo ""
  echo "Upload complete!"
fi
