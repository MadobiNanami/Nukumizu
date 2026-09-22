#!/usr/bin/env bash
#
# Builds the Windows (amd64) binary.
#
# The Vue console is built first and embedded into the binary (web/dist, see
# web/embed.go), so the executable serves the whole frontend on its own —
# neither frontend/ nor web/dist/ is needed where it runs.
set -euo pipefail

# Build from the repository root, however the script was invoked.
cd "$(dirname "$0")"

echo "Building frontend..."
(
    cd frontend
    # node_modules is gitignored, so a fresh checkout (CI included) installs
    # from the lockfile; a warm tree only rebuilds.
    if [ ! -d node_modules ]; then
        npm ci
    fi
    npm run build
)

# go:embed on web/dist fails anyway, but this names the real problem.
if [ ! -f web/dist/index.html ]; then
    echo "Frontend build produced no web/dist/index.html" >&2
    exit 1
fi

echo "Building for Windows (amd64)..."

COMMIT=$(git rev-parse --short HEAD)
BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

export GOOS=windows
export GOARCH=amd64
export CGO_ENABLED=0

OUTPUT=nukumizu-windows-amd64.exe

echo "Commit: $COMMIT"
echo "BuildDate: $BUILD_DATE"
echo "Output: $OUTPUT"

go build -ldflags "-X main.BuildTime=$BUILD_DATE -X main.CommitHash=$COMMIT" -o "$OUTPUT" .

echo "Build succeeded: $OUTPUT"
