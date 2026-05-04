#!/bin/bash
# Copyright 2026 Jasen Minton
#
# SPDX-License-Identifier: Apache-2.0

# The name of your output binary
APP_NAME="pattyGraph"

# Platforms you want to build for
platforms=("linux/amd64" "linux/arm64")

LD_FLAGS='-s -w'

if [ "$1" = "--debug" ] || [ "$1" = "--profile" ]; then
    LD_FLAGS=''
fi


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

    echo "Building for $GOOS/$GOARCH in directory ./dist/$output_dir..."
    env GOOS=$GOOS GOARCH=$GOARCH go build -ldflags="$LD_FLAGS" -o "$output_name" .  
    if [ $? -ne 0 ]; then
        echo "An error occurred while building for $platform"
        exit 1
    fi
done

echo "Build complete!"
