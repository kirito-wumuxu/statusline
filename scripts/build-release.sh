#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:-v0.1.0}"
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

  echo "Compiling ${GOOS}/${GOARCH}..."
  CGO_ENABLED=0 GOOS="${GOOS}" GOARCH="${GOARCH}" go build -ldflags="-s -w" -o "${STAGE_DIR}/statusline${EXT}" "${ROOT_DIR}/cmd/statusline"
  CGO_ENABLED=0 GOOS="${GOOS}" GOARCH="${GOARCH}" go build -ldflags="-s -w" -o "${STAGE_DIR}/statusline-server${EXT}" "${ROOT_DIR}/cmd/statusline-server"
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
