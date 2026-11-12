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

func factoidCoverageArgs(name string) []string {
	switch strings.ToLower(name) {
	case "print":
		return []string{"coverage note"}
	case "interesting.peakcontention":
		return []string{"Words", "candidate", "reduce scale"}
	case "interesting.peakretirement":
		return []string{"Words", "retired"}
	case "settings.peak-limit-clamped":
		return []string{"30", "25"}
	case "matcher":
		return []string{BotsMatcherName}
	default:
		return nil
	}
}

func generateAllRegisteredFactoids(t *testing.T, g *FactoidGenerator) map[string]string {
	t.Helper()
	generated := make(map[string]string, len(g.facts))
	for _, fact := range g.facts {
		generated[fact.Name] = fact.Generate(factoidCoverageArgs(fact.Name))
	}
	return generated
}

func TestCoverageFactoidCatalogGenerators(t *testing.T) {
	setupMonitorPipelineTestGraph()
	startTime = time.Now().Add(-90 * time.Minute)
	logLoadDuration = 750 * time.Millisecond
	logLoadLinecount = 12_345
	logLoadIntervalCount = 9
	startBytesRead = 2 * 1024 * 1024

	baseline := generateAllRegisteredFactoids(t, facts)
	if baseline["settings.push"] == "" || baseline["version.pattyGraph"] == "" {
		t.Fatal("baseline catalog generation omitted stable factoids")
	}
	if baseline["metrics.marked"] != "" || baseline["matcher.injected"] != "" {
		t.Fatal("conditional factoid appeared without supporting state")
	}

	PattyGraph.intervalLines = 100
	PattyGraph.totalLines = 10_000
	PattyGraph.totalBytes = 8 * 1024 * 1024
	PattyGraph.intervalsCompleted = 4
	PattyGraph.overallMin = 2
	PattyGraph.overallMax = 80
	PattyGraph.linesMatcher.matchCountsMap["marked"] = 25
	PattyGraph.linesMatcher.history = []int{50, 100, 75}
	PattyGraph.bytesMatcher.history = []int{1024, 2048, 4096}
	PattyGraph.errsMatcher.history = []int{0, 2, 5}
	botsMigrated = 3

	poolGets = 20
	poolNews = 5
	poolReturns = 18
	poolGetsMap = map[int]uint64{1: 5, 2: 10, 3: 2}
	poolGetsPerMatcherMap = map[uint64]uint64{2: 7, 4: 3}
	uaCardinalityMap = map[int]uint64{16: 60, 14: 20, 12: 10}

	PattyGraph.wordsMatcher.wordFrequency["one"] = &WordStats{primeFlux: 1}
	PattyGraph.wordsMatcher.wordFrequency["many"] = &WordStats{primeFlux: 4}
	PattyGraph.refsMatcher.wordFrequency["single"] = &WordStats{primeFlux: 1}
	PattyGraph.ipsMatcher.wordFrequency["192.0.2.1"] = &WordStats{primeFlux: 1}
	PattyGraph.wordsMatcher.peakWords = []string{"catalog", "health"}
	PattyGraph.refsMatcher.peakWords = []string{"search"}
	PattyGraph.wordsMatcher.peakWordCounts = []int{1, 2, 3}
	PattyGraph.wordsMatcher.totalPeakCounts = 6
	PattyGraph.ipsMatcher.wordStatsCreated = 120
	PattyGraph.wordsMatcher.wordStatsCreated = 60
	PattyGraph.refsMatcher.wordStatsCreated = 30
	PattyGraph.ipsMatcher.ipScratch.prefixToIPs["192.0.2"] = map[string]struct{}{
		"192.0.2.1": {},
		"192.0.2.2": {},
	}
	PattyGraph.ipsMatcher.ipScratch.activePrefixesCountMetric = 1

	dir := t.TempDir()
	logPath := filepath.Join(dir, "access.log")
	if err := os.WriteFile(logPath, []byte("line\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	PattyGraph.filePath = logPath
	PattyGraph.pattyConfig.setSaveDir(dir)
	generateSidecarJSONL = true
	enableControlFile = true
	sidecarWriteFailures = 1

	populated := generateAllRegisteredFactoids(t, facts)
	for _, name := range []string{
		"metrics.poolPerMatcher",
		"metrics.pool",
		"metrics.UABands",
		"metrics.marked",
		"matcher.injected",
		"interesting.oneHitWords",
		"interesting.oneHitRefs",
		"interesting.oneHitIPs",
		"mem.newEntries",
	} {
		if populated[name] == "" {
			t.Errorf("populated factoid %q returned empty output", name)
		}
	}
}

func TestCoverageFactoidArgumentValidation(t *testing.T) {
	setupMonitorPipelineTestGraph()
	checks := []struct {
		name string
		args []string
	}{
		{"print", nil},
		{"print", []string{""}},
		{"interesting.peakContention", []string{"short"}},
		{"interesting.peakRetirement", []string{"short"}},
		{"settings.peak-limit-clamped", []string{"short"}},
	}
	for _, tc := range checks {
		fact := factoidByName[strings.ToLower(tc.name)]
		if fact == nil {
			t.Fatalf("missing factoid %q", tc.name)
		}
		if got := fact.Generate(tc.args); got != "" {
			t.Errorf("factoid %q accepted invalid arguments: %q", tc.name, got)
		}
	}
}

func TestCoverageTickerRendering(t *testing.T) {
	setupMonitorPipelineTestGraph()
	doRandomFact = false
	tickerBuffer = ""
	tickerVisibleOffset = 0

	first := currentTickerText()
	second := currentTickerText()
	if first == "" || second == "" || tickerVisibleOffset != 6 {
		t.Fatalf("ticker did not advance: %q %q offset=%d", first, second, tickerVisibleOffset)
	}
}
