#!/bin/bash
# Copyright 2026 Jasen Minton
#
# SPDX-License-Identifier: Apache-2.0
#
# This srcipt is meant for distribution specific binary constructions.


# The name of your output binary
APP_NAME="pattyGraph"

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

if [[ "$RAW_VERSION" == v* ]]; then
    VERSION="$RAW_VERSION"
else
    VERSION="v$RAW_VERSION"
fi

# Platforms you want to build for
platforms=("linux/amd64" "linux/arm64")

LD_FLAGS='-s -w'

if [ "$1" = "--debug" ] || [ "$1" = "--profile" ]; then
    LD_FLAGS=''
fi

echo "Using ${APP_NAME} version $VERSION from util.go"

# Build binaries
for platform in "${platforms[@]}"; do
    IFS="/" read -r GOOS GOARCH <<< "$platform"
    
    # Create a directory for each platform
    output_dir="dist/${GOOS}-${GOARCH}"
    mkdir -p "$output_dir"
    
    # Set the output name, adding .exe for Windows builds
    output_name="$output_dir/$APP_NAME"
    if [ "$GOOS" = "windows" ]; then
        output_name+=".exe"
    fi

    echo "Building for $GOOS/$GOARCH in directory ./$output_dir..."
    env GOOS=$GOOS GOARCH=$GOARCH go build -ldflags="$LD_FLAGS" -o "$output_name" .  
    if [ $? -ne 0 ]; then
        echo "An error occurred while building for $platform"
        exit 1
    fi

    archive_name="${output_dir}/${APP_NAME}-${VERSION}-${GOOS}-${GOARCH}.tar.gz"
    echo "Creating release archive ./$archive_name..."
    tar -C "$output_dir" -czf "$archive_name" "$(basename "$output_name")"
    if [ $? -ne 0 ]; then
        echo "An error occurred while creating archive for $platform"
        exit 1
    fi
done

echo "Build complete!"
