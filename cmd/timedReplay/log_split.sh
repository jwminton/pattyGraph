#!/bin/bash
# Copyright 2026 Jasen Minton
#
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

usage() {
    echo "Usage: $0 <seed_mb> <log_file>" >&2
    echo "Creates <log_file>.seed and <log_file>.replay in the current directory" >&2
}

if [ "$#" -ne 2 ]; then
    usage
    exit 1
fi

seed_mb="$1"
input_file="$2"

if ! [[ "$seed_mb" =~ ^[0-9]+$ ]] || [ "$seed_mb" -le 0 ]; then
    echo "error: seed_mb must be a positive integer" >&2
    usage
    exit 1
fi

if [ ! -f "$input_file" ]; then
    echo "error: file not found: $input_file" >&2
    exit 1
fi

base_name="$(basename "$input_file")"
seed_file="${base_name}.seed"
replay_file="${base_name}.replay"

file_size="$(wc -c < "$input_file")"
seed_bytes=$((seed_mb * 1024 * 1024))
mb_bytes=$((1024 * 1024))
probe_bytes="$mb_bytes"

echo "Input: $input_file"
echo "Size:  $file_size bytes"
echo "Seed:  $seed_bytes bytes (${seed_mb} MB)"

if [ "$seed_bytes" -ge "$file_size" ]; then
    split_at="$file_size"
else
    target_offset="$seed_bytes"
    # Move forward to the next newline so the replay file starts on a full log line.
    newline_offset="$(
        dd if="$input_file" bs=1M skip="$target_offset" count="$probe_bytes" iflag=skip_bytes,count_bytes status=none |
            perl -0777 -ne '$i = index($_, "\n"); if ($i >= 0) { print $i }'
    )"
    if [ -z "$newline_offset" ]; then
        echo "warn: no newline found within $probe_bytes bytes of split point; splitting at exact byte offset" >&2
        split_at="$target_offset"
    else
        split_at=$((target_offset + newline_offset + 1))
        if [ "$split_at" -gt "$file_size" ]; then
            split_at="$file_size"
        fi
    fi
fi

top_bytes=$((file_size - split_at))

echo "Split: $split_at bytes"
echo "Seed bytes:   $split_at"
echo "Replay bytes: $top_bytes"
echo "Writing $seed_file..."
if [ "$split_at" -eq 0 ]; then
    : > "$seed_file"
else
    dd if="$input_file" of="$seed_file" bs=1M count="$split_at" iflag=count_bytes status=progress
fi

echo "Writing $replay_file..."
if [ "$top_bytes" -eq 0 ]; then
    : > "$replay_file"
else
    dd if="$input_file" of="$replay_file" bs=1M skip="$split_at" iflag=skip_bytes status=progress
fi

echo "Created:"
echo "  $seed_file"
echo "  $replay_file"
echo
echo "Seed current access log:"
echo "  cp $seed_file access.log"
echo
echo "Replay remaining log:"
echo "  go run ./cmd/timedReplay -file $replay_file >> access.log"
