// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"strings"
	"testing"
)

func TestCoverageMatcherSparklineAndDetailRendering(t *testing.T) {
	setupMonitorPipelineTestGraph()
	PattyGraph.overallMin = 0
	PattyGraph.overallMax = 100
	lastMonitorMaxBuf.Push(80)

	m := NewPredicateMatcher("coverage")
	m.color = "[orange]"
	m.history = []int{5, 20, 60, 30}
	m.intervalCount = 75
	m.lastIntervalCount = 60
	m.matchCountsMap = map[string]int{"alpha": 7, "beta": 7, "zero": 0}
	m.ipGroupsMap = map[string]int{"192.0.": 12}
	m.ipGroupsCountsMap = map[string]int{"192.0.": 18}

	if m.generateSparkline(0, 100) == "" || !strings.Contains(m.renderSparklineRow(), "coverage") {
		t.Fatal("matcher sparkline row omitted expected content")
	}
	PattyGraph.selectedMatcher = m
	if !strings.Contains(m.displayColor(), PattyHighlight) {
		t.Fatal("selected matcher did not use highlight color")
	}

	for mode, glyph := range []string{" ", "·", ":"} {
		m.displayMatchMode = mode
		m.detailListingCache = ""
		if m.expandGlyph() != glyph {
			t.Fatalf("mode %d glyph = %q, want %q", mode, m.expandGlyph(), glyph)
		}
		if got := m.renderDetailListing(); !strings.Contains(got, "coverage") {
			t.Fatalf("mode %d detail listing omitted matcher name", mode)
		}
	}
	m.displayMatchMode = 99
	if m.expandGlyph() != "!" {
		t.Fatal("unexpected mode did not use defensive glyph")
	}
	m.cycleDisplayMatchMode()

	m.markAsAddedMatcher()
	m.displayMatchMode = 1
	m.detailListingCache = ""
	if got := m.renderDetailListing(); !strings.Contains(got, "192.0.*.*") {
		t.Fatalf("grouped matcher listing omitted prefix: %q", got)
	}
}

func TestCoverageMatcherSortAndSystemRows(t *testing.T) {
	setupMonitorPipelineTestGraph()
	PattyGraph.intervalLines = 100

	standard := []matchEntry{{"z", 1}, {"b", 3}, {"a", 3}}
	NewPredicateMatcher("sort").standardEntrySort(standard)
	if standard[0].match != "a" || standard[2].match != "z" {
		t.Fatalf("standard sort mismatch: %#v", standard)
	}
	bytesEntries := []matchEntry{{"unknown", 1}, {">1M", 1}, {"<100B", 1}, {"alpha", 1}}
	bytesEntrySort(bytesEntries)
	if bytesEntries[0].match != "<100B" || bytesEntries[1].match != ">1M" {
		t.Fatalf("bytes sort mismatch: %#v", bytesEntries)
	}
	lineEntries := []matchEntry{{"zzz", 1}, {" b16", 1}, {"marked", 1}, {"aaa", 1}}
	linesEntrySort(lineEntries)
	if lineEntries[0].match != "marked" || lineEntries[1].match != " b16" {
		t.Fatalf("lines sort mismatch: %#v", lineEntries)
	}

	PattyGraph.bytesMatcher.intervalCount = 4096
	PattyGraph.bytesMatcher.matchCountsMap = map[string]int{"<100B": 20, "----": 1, ">1M": 5}
	PattyGraph.bytesMatcher.displayMatchMode = 1
	PattyGraph.bytesMatcher.detailListingCache = ""
	if got := PattyGraph.bytesMatcher.renderDetailListing(); !strings.Contains(got, "<100B") {
		t.Fatalf("bytes detail omitted size band: %q", got)
	}
	PattyGraph.linesMatcher.intervalCount = 100
	PattyGraph.linesMatcher.matchCountsMap = map[string]int{"marked": 70, " b16": 55, "other": 3, "----": 1}
	for _, mode := range []int{1, 2} {
		PattyGraph.linesMatcher.displayMatchMode = mode
		PattyGraph.linesMatcher.detailListingCache = ""
		if got := PattyGraph.linesMatcher.renderDetailListing(); !strings.Contains(got, "marked") {
			t.Fatalf("lines detail omitted marked row in mode %d: %q", mode, got)
		}
	}

	for _, pct := range []float64{0, 10, 25, 35, 45, 75, 100} {
		if len(renderBarBraille(pct)) == 0 || !strings.Contains(renderColoredBar(pct), "[") {
			t.Fatalf("bar rendering failed for %.1f", pct)
		}
	}
}

func TestCoverageMatcherHistoryAndTagging(t *testing.T) {
	setupMonitorPipelineTestGraph()
	m := NewPredicateMatcher("tagger")
	if m.previousValue() != 0 || m.previousAverage() != 0 || m.changeRate() != 1.0 {
		t.Fatal("empty matcher history defaults changed")
	}
	m.intervalCount = 5
	if m.changeRate() <= 1 || m.lastCycleCount != 5 {
		t.Fatal("initial positive change was not detected")
	}
	m.lastIntervalCount = 60
	m.intervalCount = 10
	_ = m.changeRate()
	m.intervalCount = 2
	_ = m.changeRate()
	m.history = []int{1, 2, 3, 4, 5, 6}
	if m.previousValue() != 6 || m.previousAverage() != 4 {
		t.Fatalf("history summaries mismatch: %d %.1f", m.previousValue(), m.previousAverage())
	}

	m.markAsAddedMatcher()
	m.matchCountsMap["192.0.2.4"] = 1
	if color, ok := m.tagIp("192.0.2.4"); !ok || color != m.color {
		t.Fatal("remembered IP was not tagged")
	}
	if _, ok := m.tagIp("192.0.2.5"); ok {
		t.Fatal("unknown IP was tagged")
	}
	currentLine.ip = "192.0.2.4"
	if !m.defaultTagIpAction() || currentLine.captureMatcher != "tagger" {
		t.Fatal("default IP tag action did not capture matcher")
	}
	m.purgeHistory()
	if len(m.history) != 0 {
		t.Fatal("history purge retained entries")
	}
	m.prePush()

	if CodeMatcher("codes", []string{"404"}) == nil || StartsWithMatcher("prefix", []string{"/api"}) == nil {
		t.Fatal("matcher constructors returned nil")
	}
}

func TestCoverageMatcherSourceLinesAndStats(t *testing.T) {
	setupMonitorPipelineTestGraph()
	m := NewPredicateMatcher("source")
	m.color = "[green]"
	m.firstMatchLine = "first"
	m.intervalMatchLine = "interval"
	m.lastMatchLine = "last"
	if !strings.Contains(m.displayFirstLine(), "first") ||
		!strings.Contains(m.displayFirstIntervalLine(), "interval") ||
		!strings.Contains(m.displayLastLine(), "last") {
		t.Fatal("matcher source-line display omitted retained lines")
	}
	m.firstMatchLine = ""
	m.intervalMatchLine = ""
	m.lastMatchLine = ""
	_ = m.displayFirstLine()
	_ = m.displayFirstIntervalLine()
	_ = m.displayLastLine()

	m.history = []int{1, 10, 3, 6}
	high, low, avg := m.factoidMaxMinAvg()
	if high != 10 || low != 1 || avg != 5 {
		t.Fatalf("factoid stats = %d/%d/%.1f", high, low, avg)
	}
	if !strings.Contains(m.pushStatsMsg(), "source") {
		t.Fatal("stats message omitted matcher name")
	}
	if got := PattyGraph.bytesMatcher.pushStatsMsg(); got != "" {
		t.Fatalf("bytes matcher unexpectedly emitted stats: %q", got)
	}
}
