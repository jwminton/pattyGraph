// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"fmt"
	"testing"
	"time"
)

func TestDefaultSidecarOptionsStayCompact(t *testing.T) {
	opts := DefaultSidecarOptions()

	if opts.TopLimit != defaultSidecarTopLimit {
		t.Fatalf("TopLimit = %d, want %d", opts.TopLimit, defaultSidecarTopLimit)
	}
	if opts.FactoidLimit != 8 {
		t.Fatalf("FactoidLimit = %d, want 8", opts.FactoidLimit)
	}
	if opts.IncludeHistories {
		t.Fatal("IncludeHistories = true, want false")
	}
	if opts.IncludeSourceLines {
		t.Fatal("IncludeSourceLines = true, want false")
	}
	if !opts.IncludeMatcherKeys {
		t.Fatal("IncludeMatcherKeys = false, want true")
	}
}

func TestSidecarSnapshotUsesMonitorLogTime(t *testing.T) {
	setupMonitorPipelineTestGraph()
	want := time.Date(2026, time.May, 13, 14, 22, 31, 0, time.FixedZone("PDT", -7*60*60))
	PattyGraph.logtime = want

	opts := DefaultSidecarOptions()
	opts.FactoidLimit = 0
	snap := PattyGraph.SidecarSnapshot(opts)

	if !snap.Timestamp.Equal(want) {
		t.Fatalf("Timestamp = %s, want %s", snap.Timestamp, want)
	}
	if !snap.LogTime.Equal(want) {
		t.Fatalf("LogTime = %s, want %s", snap.LogTime, want)
	}
}

func TestSidecarWordEntriesAreCappedSortedAndRanked(t *testing.T) {
	setupMonitorPipelineTestGraph()
	words := WordMatcherFactory("words")
	opts := DefaultSidecarOptions()
	limit := 5

	for i := 0; i < limit+3; i++ {
		stats := newWordStats()
		stats.count = i + 1
		stats.primeFlux = i * 2
		words.wordFrequency[fmt.Sprintf("word%02d", i)] = stats
	}

	entries := sidecarWordEntries(words, limit, opts)

	if len(entries) != limit {
		t.Fatalf("len(entries) = %d, want %d", len(entries), limit)
	}
	for i, entry := range entries {
		wantRank := i + 1
		if entry.Rank != wantRank {
			t.Fatalf("entries[%d].Rank = %d, want %d", i, entry.Rank, wantRank)
		}
		if i > 0 && entry.Score > entries[i-1].Score {
			t.Fatalf("entries not sorted by descending score: %v then %v", entries[i-1], entry)
		}
	}
	if entries[0].Key != "word07" {
		t.Fatalf("top key = %q, want word07", entries[0].Key)
	}
	if entries[len(entries)-1].Key != "word03" {
		t.Fatalf("last capped key = %q, want word03", entries[len(entries)-1].Key)
	}
}
