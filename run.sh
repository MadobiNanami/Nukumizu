#!/usr/bin/env bash
#
# Runs the server from source, with the build metadata stamped in.
#
# The console is embedded in the binary (web/embed.go), so compiling without
# web/dist fails. Say that plainly rather than leaving go:embed's error.
set -euo pipefail

# Run from the repository root, however the script was invoked.
cd "$(dirname "$0")"

if [ ! -f web/dist/index.html ]; then
    echo "The web console is not built: web/dist is missing." >&2
    echo "Run build-linux-x86_64.sh once, or \"npm run build\" in frontend/." >&2
    exit 1
fi

# Get git commit hash (shortened to 7 characters, can also use full)
COMMIT=$(git rev-parse --short HEAD)

# Get UTC time
BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

# Build -ldflags
LDFLAGS="-X main.BuildTime=$BUILD_DATE -X main.CommitHash=$COMMIT"
go run -ldflags "$LDFLAGS" .
