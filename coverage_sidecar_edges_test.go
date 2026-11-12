// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCoverageSidecarConvenienceWriters(t *testing.T) {
	setupMonitorPipelineTestGraph()
	dir := t.TempDir()
	PattyGraph.pattyConfig.setSaveDir(dir)
	PattyGraph.pattyConfig.setJSONFile("coverage.jsonl")
	PattyGraph.logtime = time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	PattyGraph.totalLines = 10
	PattyGraph.intervalLines = 3

	after := PattyGraph.SidecarSnapshotAfterPush()
	if after.EventType == "" {
		t.Fatal("after-push snapshot omitted event type")
	}
	path := filepath.Join(dir, "events.jsonl")
	if err := PattyGraph.WriteSidecarSnapshotJSONL(SidecarOptions{}, path); err != nil {
		t.Fatal(err)
	}
	transition := AlertTransition{
		Status:      AlertStatusTriggered,
		MatcherName: "errs",
		Direction:   AlertDirectionAbove,
		Value:       5,
		Threshold:   3,
	}
	if err := PattyGraph.WriteSidecarAlertJSONL(transition, path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "alert") || !strings.Contains(string(data), "interval") {
		t.Fatalf("sidecar convenience output mismatch: %q %v", data, err)
	}
}

func TestCoverageSidecarOptionAndCopyEdges(t *testing.T) {
	opts := normalizeSidecarOptions(SidecarOptions{})
	if opts.TopLimit <= 0 || opts.Phase != SidecarPhaseBeforePush {
		t.Fatalf("default sidecar options were not applied: %#v", opts)
	}
	custom := SidecarOptions{TopLimit: 2, Phase: SidecarPhaseAfterPush}
	if got := normalizeSidecarOptions(custom); got.TopLimit != 2 || got.Phase != SidecarPhaseAfterPush {
		t.Fatalf("custom sidecar options changed: %#v", got)
	}
	if copyUint64Map(nil) != nil {
		t.Fatal("nil uint map copy was nonnil")
	}
	source := map[int]uint64{1: 2, 3: 4}
	copyMap := copyUint64Map(source)
	copyMap[1] = 99
	if source[1] != 2 {
		t.Fatal("uint map copy shared storage")
	}
	if representativeLogLine("", "first", "second") != "first" || representativeLogLine() != "" {
		t.Fatal("representative source selection changed")
	}
	if lineSourceLogLine(nil) != "" || lineSourceLogLine(&lineSource{logLine: "line"}) != "line" {
		t.Fatal("line source extraction changed")
	}
}

func TestCoverageSidecarHelpText(t *testing.T) {
	if !strings.Contains(sidecarHelpText(), "JSONL") || !strings.Contains(aiHelpText(), "pattyGraph") {
		t.Fatal("sidecar or AI help omitted its subject")
	}
}
