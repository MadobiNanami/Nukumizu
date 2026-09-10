#!/usr/bin/env bash
#
# Builds the Linux (amd64) binary.
#   ./build-linux-x86_64.sh              backend only
#   ./build-linux-x86_64.sh --frontend   also build the Vue frontend
#
# The frontend is needed at runtime: web/ serves it from frontend/dist.
set -euo pipefail

# Build from the repository root, however the script was invoked.
cd "$(dirname "$0")"

BUILD_FRONTEND=0
if [ "${1:-}" = "--frontend" ]; then
    BUILD_FRONTEND=1
fi

if [ "$BUILD_FRONTEND" = "1" ]; then
    echo "Building frontend..."
    (
        cd frontend
        npm ci
        npm run build
    )
fi

echo "Building for Linux (amd64)..."

COMMIT=$(git rev-parse --short HEAD)
BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

export GOOS=linux
export GOARCH=amd64
export CGO_ENABLED=0

OUTPUT=nukumizu-linux-amd64

echo "Commit: $COMMIT"
echo "BuildDate: $BUILD_DATE"
echo "Output: $OUTPUT"

go build -ldflags "-X main.BuildTime=$BUILD_DATE -X main.CommitHash=$COMMIT" -o "$OUTPUT" .

echo "Build succeeded: $OUTPUT"
