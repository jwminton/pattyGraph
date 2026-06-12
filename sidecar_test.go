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

	if SidecarSchemaVersion != 3 {
		t.Fatalf("SidecarSchemaVersion = %d, want 3", SidecarSchemaVersion)
	}

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

func TestSidecarWordEntryMarkedFields(t *testing.T) {
	setupMonitorPipelineTestGraph()
	words := WordMatcherFactory("words")
	opts := DefaultSidecarOptions()

	markedStats := newWordStats()
	markedStats.source.captureColor = "[red]"
	markedStats.source.captureMatcher = "Bots"
	marked := sidecarWordEntry(words, "marked", markedStats, opts)
	if marked.MarkedState != SidecarMarkedStateMarked {
		t.Fatalf("marked state = %q, want %q", marked.MarkedState, SidecarMarkedStateMarked)
	}
	if marked.MarkedByMatcher != "Bots" {
		t.Fatalf("marked by = %q, want Bots", marked.MarkedByMatcher)
	}

	unmarkedStats := newWordStats()
	unmarkedStats.source.captureColor = ""
	unmarkedStats.source.captureMatcher = ""
	unmarked := sidecarWordEntry(words, "unmarked", unmarkedStats, opts)
	if unmarked.MarkedState != SidecarMarkedStateUnmarked {
		t.Fatalf("unmarked state = %q, want %q", unmarked.MarkedState, SidecarMarkedStateUnmarked)
	}
	if unmarked.MarkedByMatcher != "" {
		t.Fatalf("unmarked by = %q, want empty", unmarked.MarkedByMatcher)
	}
}

func TestSidecarSelectedContextIncludesSelectedWordLines(t *testing.T) {
	setupMonitorPipelineTestGraph()
	refs := WordMatcherFactory("refs")
	stats := newWordStats()
	firstLine := `192.0.2.1 - - [22/Jan/2019:05:24:59 +0330] "GET /first HTTP/1.1" 200 3710 "https://example.test/start?page=1" "FirstAgent/1.0" "-"`
	firstIntervalLine := `192.0.2.2 - - [22/Jan/2019:05:25:10 +0330] "POST /interval HTTP/1.1" 404 12 "https://example.test/interval?page=1" "IntervalAgent/2.0" "-"`
	lastLine := `192.0.2.3 - - [22/Jan/2019:05:26:20 +0330] "GET /last HTTP/1.1" 499 0 "https://example.test/last?page=1" "LastAgent/3.0" "-"`
	stats.source.logLine = firstLine
	stats.source.ip = "192.0.2.1"
	stats.source.ipPrefix = "192.0.2"
	stats.source.request = "GET /first HTTP/1.1"
	stats.source.respCode = "200"
	stats.source.bytesValue = 3710
	stats.source.referer = "https://example.test/start?page=1"
	stats.source.userAgent = "FirstAgent/1.0"
	stats.firstIntervalLogLine = firstIntervalLine
	stats.lastLogLine = lastLine
	refs.wordFrequency["torob.com"] = stats
	refs.selectedKey = "torob.com"
	PattyGraph.selectedInterestingMatcher = refs

	selected := sidecarSelectedContext(PattyGraph)
	if selected.InterestingMatcher != "refs" {
		t.Fatalf("interesting matcher = %q, want refs", selected.InterestingMatcher)
	}
	if selected.InterestingKey != "torob.com" {
		t.Fatalf("interesting key = %q, want torob.com", selected.InterestingKey)
	}
	if selected.FirstSource == nil || selected.FirstSource.Request != "GET /first HTTP/1.1" || selected.FirstSource.BytesValue != 3710 {
		t.Fatalf("first source = %#v", selected.FirstSource)
	}
	if selected.FirstSource.LogLine != firstLine {
		t.Fatalf("first source log line = %q", selected.FirstSource.LogLine)
	}
	if selected.FirstIntervalSource == nil || selected.FirstIntervalSource.ResponseCode != "404" || selected.FirstIntervalSource.Request != "POST /interval HTTP/1.1" {
		t.Fatalf("first interval source = %#v", selected.FirstIntervalSource)
	}
	if selected.FirstIntervalSource.LogLine != firstIntervalLine {
		t.Fatalf("first interval source log line = %q", selected.FirstIntervalSource.LogLine)
	}
	if selected.LastSource == nil || selected.LastSource.ResponseCode != "499" || selected.LastSource.BytesValue != 0 {
		t.Fatalf("last source = %#v", selected.LastSource)
	}
	if selected.LastSource.LogLine != lastLine {
		t.Fatalf("last source log line = %q", selected.LastSource.LogLine)
	}
}

func TestSidecarSessionStartIncludesControlFileMetadata(t *testing.T) {
	setupMonitorPipelineTestGraph()
	enableControlFile = true
	PattyGraph.pattyConfig.saveDir = "/tmp/patty"

	event := PattyGraph.SidecarSessionStart()

	if !event.InlineCommandsEnabled {
		t.Fatal("InlineCommandsEnabled = false, want true")
	}
	if !event.ControlFileEnabled {
		t.Fatal("ControlFileEnabled = false, want true")
	}
	if event.ControlFilePath != "/tmp/patty/pattyControl.log" {
		t.Fatalf("ControlFilePath = %q, want /tmp/patty/pattyControl.log", event.ControlFilePath)
	}
}

func TestSidecarControlCommandIncludesControlFileMetadata(t *testing.T) {
	setupMonitorPipelineTestGraph()
	enableControlFile = true
	PattyGraph.pattyConfig.saveDir = "/tmp/patty"
	result := invokeInlineCommand("!!! add ip-91 --ips 91.99.72.15")

	event := PattyGraph.SidecarControlCommand("!!! add ip-91 --ips 91.99.72.15", "control_file", result)

	if event.EventType != SidecarEventControlCommand {
		t.Fatalf("EventType = %q, want %q", event.EventType, SidecarEventControlCommand)
	}
	if event.Source != "control_file" {
		t.Fatalf("Source = %q, want control_file", event.Source)
	}
	if event.CommandName != "add" {
		t.Fatalf("CommandName = %q, want add", event.CommandName)
	}
	if event.Status != InlineCommandStatusApplied {
		t.Fatalf("Status = %q, want %q", event.Status, InlineCommandStatusApplied)
	}
	if !event.ControlFileEnabled {
		t.Fatal("ControlFileEnabled = false, want true")
	}
	if event.ControlFilePath != "/tmp/patty/pattyControl.log" {
		t.Fatalf("ControlFilePath = %q, want /tmp/patty/pattyControl.log", event.ControlFilePath)
	}
	if event.Result["action"] != "add_matcher" {
		t.Fatalf("result action = %v, want add_matcher", event.Result["action"])
	}
}
