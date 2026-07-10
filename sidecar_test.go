// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultSidecarOptionsStayCompact(t *testing.T) {
	opts := DefaultSidecarOptions()

	if SidecarSchemaVersion != 4 {
		t.Fatalf("SidecarSchemaVersion = %d, want 4", SidecarSchemaVersion)
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

func TestTimestampedFileIDUsesSidecarStyleUTCStem(t *testing.T) {
	stamp := time.Date(2026, time.June, 12, 1, 2, 3, 4005006, time.FixedZone("PDT", -7*60*60))
	want := fmt.Sprintf("20260612_080203_%d", os.Getpid())

	if got := timestampedFileID(stamp); got != want {
		t.Fatalf("timestampedFileID() = %q, want %q", got, want)
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

func TestSidecarDefaultPathUsesConfiguredJSONFile(t *testing.T) {
	setupMonitorPipelineTestGraph()
	PattyGraph.pattyConfig.setSaveDir("/tmp/patty")
	PattyGraph.pattyConfig.setJSONFile("current.jsonl")

	if got, want := PattyGraph.SidecarDefaultPath(), "/tmp/patty/current.jsonl"; got != want {
		t.Fatalf("SidecarDefaultPath = %q, want %q", got, want)
	}
}

func TestSidecarSessionStartTruncatesConfiguredJSONFile(t *testing.T) {
	setupMonitorPipelineTestGraph()
	dir := t.TempDir()
	PattyGraph.pattyConfig.setSaveDir(dir)
	PattyGraph.pattyConfig.setJSONFile("current.jsonl")

	if err := PattyGraph.WriteSidecarSessionStartJSONL(""); err != nil {
		t.Fatalf("first WriteSidecarSessionStartJSONL: %v", err)
	}
	if err := PattyGraph.WriteSidecarJSONL(PattyGraph.SidecarSnapshotBeforePush(), ""); err != nil {
		t.Fatalf("WriteSidecarJSONL: %v", err)
	}
	if err := PattyGraph.WriteSidecarSessionStartJSONL(""); err != nil {
		t.Fatalf("second WriteSidecarSessionStartJSONL: %v", err)
	}

	data, err := os.ReadFile(PattyGraph.SidecarDefaultPath())
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if lines := strings.Count(string(data), "\n"); lines != 1 {
		t.Fatalf("line count after truncate = %d, want 1; data=%s", lines, data)
	}
}

func TestSidecarWriteFailuresDisableJSONLOutput(t *testing.T) {
	silenceExpectedLogs(t)
	generateSidecarJSONL = true
	sidecarWriteFailures = 0
	err := os.ErrPermission

	recordSidecarWriteResult("test", err)
	recordSidecarWriteResult("test", err)
	if !generateSidecarJSONL {
		t.Fatal("generateSidecarJSONL disabled before failure limit")
	}

	recordSidecarWriteResult("test", err)
	if generateSidecarJSONL {
		t.Fatal("generateSidecarJSONL still enabled after failure limit")
	}
}

func TestSidecarWriteSuccessResetsFailureCount(t *testing.T) {
	silenceExpectedLogs(t)
	generateSidecarJSONL = true
	sidecarWriteFailures = 0
	err := os.ErrPermission

	recordSidecarWriteResult("test", err)
	recordSidecarWriteResult("test", nil)
	recordSidecarWriteResult("test", err)
	recordSidecarWriteResult("test", err)

	if !generateSidecarJSONL {
		t.Fatal("generateSidecarJSONL disabled despite success resetting failure count")
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

func TestSidecarControlCommandPreservesInvalidArgumentFields(t *testing.T) {
	setupMonitorPipelineTestGraph()
	path := filepath.Join(t.TempDir(), "pattyLog.jsonl")
	result := invokeInlineCommand("!!! flux 99")

	if err := PattyGraph.WriteSidecarControlCommandJSONL("!!! flux 99", "control_file", result, path); err != nil {
		t.Fatalf("WriteSidecarControlCommandJSONL: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var event SidecarControlCommand
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if event.Status != InlineCommandStatusRejected {
		t.Fatalf("status = %q, want %q", event.Status, InlineCommandStatusRejected)
	}
	if event.Result["error_kind"] != "invalid_argument" ||
		event.Result["argument"] != "flux" ||
		event.Result["value"] != "99" ||
		event.Result["error"] != "flux must be between 1 and 10" {
		t.Fatalf("invalid argument result = %#v", event.Result)
	}
}

func TestSidecarControlCommandWrittenWhenJSONOffDisablesOutput(t *testing.T) {
	setupMonitorPipelineTestGraph()
	dir := t.TempDir()
	path := filepath.Join(dir, "pattyLog.jsonl")
	PattyGraph.pattyConfig.setJSONFile(path)
	generateSidecarJSONL = true

	command := "!!! json off"
	wasSidecarEnabled := generateSidecarJSONL
	result := invokeInlineCommand(command)
	if generateSidecarJSONL {
		t.Fatal("json off did not disable JSONL output")
	}
	if wasSidecarEnabled || generateSidecarJSONL {
		if err := PattyGraph.WriteSidecarControlCommandJSONL(command, "control_file", result, ""); err != nil {
			t.Fatalf("WriteSidecarControlCommandJSONL: %v", err)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), `"event_type":"control_command"`) {
		t.Fatalf("control command event missing: %s", data)
	}
	if !strings.Contains(string(data), `"command":"!!! json off"`) {
		t.Fatalf("json off command missing: %s", data)
	}
	if !strings.Contains(string(data), `"enabled":false`) {
		t.Fatalf("enabled=false missing: %s", data)
	}
}

func TestSidecarAlertUsesTransitionFields(t *testing.T) {
	setupMonitorPipelineTestGraph()
	transition := AlertTransition{
		Status:       AlertStatusTriggered,
		MatcherName:  "errs",
		Direction:    AlertDirectionAbove,
		Value:        83,
		Threshold:    50,
		FluxDepth:    3,
		Streak:       3,
		Interval:     42,
		CurrentCycle: 60,
	}

	event := PattyGraph.SidecarAlert(transition)

	if event.EventType != SidecarEventAlert {
		t.Fatalf("EventType = %q, want %q", event.EventType, SidecarEventAlert)
	}
	if event.Status != AlertStatusTriggered || event.Matcher != "errs" || event.Direction != AlertDirectionAbove {
		t.Fatalf("alert identity = %#v", event)
	}
	if event.Value != 83 || event.Threshold != 50 || event.FluxDepth != 3 || event.Streak != 3 {
		t.Fatalf("alert numeric fields = %#v", event)
	}
	if event.Interval != 42 || event.CurrentCycle != 60 {
		t.Fatalf("alert timing fields = %#v", event)
	}
}

func TestBackgroundFactoidsDoNotConsumeStartupWelcome(t *testing.T) {
	oldDoRandom := doRandomFact
	doRandomFact = false
	t.Cleanup(func() { doRandomFact = oldDoRandom })

	g := NewFactoidGenerator()
	if len(g.forced) == 0 {
		t.Fatal("forced startup factoids = 0, want welcome queued")
	}

	background, _, _ := g.NextBackground()
	if strings.TrimSpace(background) != "" {
		t.Fatalf("background factoid = %q, want blank when random is disabled", background)
	}
	if len(g.forced) == 0 {
		t.Fatal("NextBackground consumed forced startup factoid")
	}

	welcome, _, _ := g.Next()
	if !strings.Contains(welcome, PattyGraphVersion) && !strings.Contains(welcome, "▁▂▃▄▅▆▇█") {
		t.Fatalf("first normal factoid = %q, want welcome", welcome)
	}
}
