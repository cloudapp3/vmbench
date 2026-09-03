#!/usr/bin/env bash
#
# vmbench 构建脚本
# 将构建产物输出到 /root/temp 目录
#
set -euo pipefail

# ── 配置 ──
PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
OUTPUT_DIR="/root/temp"
BINARY_NAME="vmbench"

# ── 版本信息 ──
VERSION="$(git -C "$PROJECT_DIR" describe --tags --always --dirty 2>/dev/null || echo "dev")"
COMMIT="$(git -C "$PROJECT_DIR" rev-parse --short HEAD 2>/dev/null || echo "unknown")"
BUILD_TIME="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"

# ── 构建 ──
echo "==> Building ${BINARY_NAME}"
echo "    Version : ${VERSION}"
echo "    Commit  : ${COMMIT}"
echo "    Time    : ${BUILD_TIME}"
echo "    Output  : ${OUTPUT_DIR}/${BINARY_NAME}"

mkdir -p "$OUTPUT_DIR"

cd "$PROJECT_DIR"
CGO_ENABLED=0 go build \
  -trimpath \
  -ldflags "-s -w \
    -X github.com/cloudapp3/vmbench.Version=${VERSION} \
    -X github.com/cloudapp3/vmbench.Commit=${COMMIT} \
    -X github.com/cloudapp3/vmbench.BuildTime=${BUILD_TIME}" \
  -o "${OUTPUT_DIR}/${BINARY_NAME}" \
  ./cmd/vmbench

echo "==> Done: $(ls -lh "${OUTPUT_DIR}/${BINARY_NAME}" | awk '{print $5}')"
"${OUTPUT_DIR}/${BINARY_NAME}" version
