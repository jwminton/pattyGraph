#!/usr/bin/env bash
# Copyright 2026 Jasen Minton
#
# SPDX-License-Identifier: Apache-2.0
#
# Build the production frontend, compile both tools, and package one archive per
# target platform. Run this script from anywhere inside a release checkout.

set -euo pipefail

ROOT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
cd "$ROOT_DIR"

APP_NAME="pattyGraph"
VIEW_NAME="pattyView"
VIEW_DIR="cmd/pattyView"

if [ ! -f util.go ]; then
    echo "Unable to determine ${APP_NAME} version: util.go was not found." >&2
    echo "compile.sh expects PattyGraphVersion to be declared in util.go, for example: PattyGraphVersion = \"0.1.3\"" >&2
    exit 1
fi

RAW_VERSION=$(sed -nE 's/^[[:space:]]*PattyGraphVersion[[:space:]]*=[[:space:]]*"([^"]+)".*/\1/p' util.go | head -n 1)
if [ -z "$RAW_VERSION" ]; then
    echo "Unable to determine ${APP_NAME} version: PattyGraphVersion was not found in util.go." >&2
    echo "compile.sh expects a string declaration like: PattyGraphVersion = \"0.1.3\"" >&2
    exit 1
fi

VIEW_VERSION=$(sed -nE 's/^[[:space:]]*"version"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/p' "$VIEW_DIR/package.json" | head -n 1)
if [ "$VIEW_VERSION" != "$RAW_VERSION" ]; then
    echo "Version mismatch: PattyGraph is $RAW_VERSION but PattyView is ${VIEW_VERSION:-unset}." >&2
    exit 1
fi

if [[ "$RAW_VERSION" == v* ]]; then
    VERSION="$RAW_VERSION"
else
    VERSION="v$RAW_VERSION"
fi

platforms=("linux/amd64" "linux/arm64")

LD_FLAGS='-s -w'

if [ "${1:-}" = "--debug" ] || [ "${1:-}" = "--profile" ]; then
    LD_FLAGS=''
fi

echo "Building ${APP_NAME} and ${VIEW_NAME} $VERSION"
echo "Installing pinned PattyView frontend dependencies..."
(
    cd "$VIEW_DIR"
    npm ci
    echo "Building PattyView frontend assets for Go embedding..."
    npm run build
)

for platform in "${platforms[@]}"; do
    IFS="/" read -r GOOS GOARCH <<< "$platform"

    output_dir="dist/${GOOS}-${GOARCH}"
    mkdir -p "$output_dir"

    graph_output="$output_dir/$APP_NAME"
    view_output="$output_dir/$VIEW_NAME"
    if [ "$GOOS" = "windows" ]; then
        graph_output+=".exe"
        view_output+=".exe"
    fi

    echo "Building ${APP_NAME} for $GOOS/$GOARCH..."
    env GOOS="$GOOS" GOARCH="$GOARCH" go build -ldflags="$LD_FLAGS" -o "$graph_output" .

    echo "Building ${VIEW_NAME} for $GOOS/$GOARCH..."
    env GOOS="$GOOS" GOARCH="$GOARCH" go build -ldflags="$LD_FLAGS" -o "$view_output" ./cmd/pattyView

    archive_name="${output_dir}/${APP_NAME}-${VERSION}-${GOOS}-${GOARCH}.tar.gz"
    echo "Creating release archive ./$archive_name..."
    tar -C "$output_dir" -czf "$archive_name" \
        "$(basename "$graph_output")" \
        "$(basename "$view_output")"
done

echo "Build complete. Each release archive contains ${APP_NAME} and ${VIEW_NAME}."
