// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"strings"
	"testing"
)

func TestCoverageTUIRulersAndTimeScale(t *testing.T) {
	ruler := fillRuler(25, false)
	if len(ruler) != 25 || !strings.HasPrefix(ruler, "1234567890") {
		t.Fatalf("single-line ruler mismatch: %q", ruler)
	}
	multi := fillRuler(20, true)
	if !strings.Contains(multi, "\n") || len(strings.Split(multi, "\n")) != 2 {
		t.Fatalf("multi-line ruler mismatch: %q", multi)
	}
	if got := columnRuler(); !strings.Contains(got, "\n") {
		t.Fatal("column ruler was not multi-line")
	}

	setupMonitorPipelineTestGraph()
	for _, index := range []int{0, 5, 15, 20, 80} {
		PattyGraph.miniWindowIndex = index
		scale := createTimeScaleWithHighlights(DefaultHistoryDepth)
		if !strings.Contains(scale, "[white]") || !strings.Contains(scale, "[default]") {
			t.Fatalf("time scale did not highlight index %d: %q", index, scale)
		}
	}
	PattyGraph.miniWindowIndex = 73
	if scale := createTimeScaleWithHighlights(15); scale == "" {
		t.Fatal("short time scale was empty")
	}

	timeScaleCache = ""
	expertMode = false
	normal := timeScaleWithHighlights()
	if normal == "" || timeScaleWithHighlights() != normal {
		t.Fatal("normal time scale was not cached")
	}
	timeScaleCache = ""
	expertMode = true
	if expert := timeScaleWithHighlights(); expert == "" || expert == normal {
		t.Fatal("expert time scale did not use its own width")
	}
}

func TestCoverageTUILayoutAndPanelHeights(t *testing.T) {
	setupMonitorPipelineTestGraph()
	PattyGraph.showTicker = false
	base := len(PattyGraph.matchers) - 1
	if got := PattyGraph.sparkPanelHeight(); got != base {
		t.Fatalf("base panel height = %d, want %d", got, base)
	}

	PattyGraph.showTicker = true
	if got := PattyGraph.sparkPanelHeight(); got != base+1 {
		t.Fatalf("ticker panel height = %d", got)
	}
	PattyGraph.selectedMatcher = PattyGraph.botsMatcher
	if got := PattyGraph.sparkPanelHeight(); got != base+3 {
		t.Fatalf("selected matcher height = %d", got)
	}
	PattyGraph.selectedMatcher = PattyGraph.changeMatcher.asMatcher()
	if got := PattyGraph.sparkPanelHeight(); got != base+1 {
		t.Fatalf("source-suppressed matcher height = %d", got)
	}
	PattyGraph.selectedMatcher = nil
	PattyGraph.selectedInterestingMatcher = PattyGraph.wordsMatcher
	PattyGraph.wordsMatcher.selectedKey = "missing"
	if got := PattyGraph.sparkPanelHeight(); got != base+4 {
		t.Fatalf("interesting selection height = %d", got)
	}
	_ = PattyGraph.sparkPanelHeightOld()
	PattyGraph.selectedInterestingMatcher = nil
	PattyGraph.selectedMatcher = PattyGraph.botsMatcher
	_ = PattyGraph.sparkPanelHeightOld()
	PattyGraph.selectedMatcher = nil
	_ = PattyGraph.sparkPanelHeightOld()

	layoutUI()
	if PattyGraph.layout == nil {
		t.Fatal("layout was not constructed")
	}
	relayoutUI()
}

func TestCoverageTUIStatusAndColorHelpers(t *testing.T) {
	setupMonitorPipelineTestGraph()
	lastMonitorMaxBuf.Reset()
	lastLinesBuf.Reset()
	lastBytesBuf.Reset()
	lastMonitorMaxBuf.Push(15)
	lastLinesBuf.Push(10)
	lastLinesBuf.Push(20)
	lastBytesBuf.Push(4096)
	lastBytesBuf.Push(2048)
	PattyGraph.secondaryView = SecondaryViewBytes
	status := PattyGraph.statusLine()
	if status == "" || !strings.Contains(status, "_") || !strings.Contains(status, upArrow) || !strings.Contains(status, downArrow) {
		t.Fatalf("status line omitted expected state: %q", status)
	}

	lastLinesBuf.Push(20)
	lastBytesBuf.Push(2048)
	if equal := PattyGraph.statusLine(); equal == "" {
		t.Fatal("equal-value status line was empty")
	}
	for view := SecondaryView(0); view < secondaryViewCount; view++ {
		if view.Glyph() == "" {
			t.Fatalf("view %d has no glyph", view)
		}
	}
	if SecondaryView(99).Glyph() != "" {
		t.Fatal("unknown secondary view returned a glyph")
	}

	if got := colorText("abc", "red"); got != "[red]a[red]b[red]c" {
		t.Fatalf("colored text = %q", got)
	}
	if colorText("", "red") != "" {
		t.Fatal("empty colored text was nonempty")
	}

	PattyGraphBuilderComplex.graphingBuilder.WriteString("graph")
	PattyGraphBuilderComplex.matcherBuilder.WriteString("matcher")
	PattyGraphBuilderComplex.Reset()
	if PattyGraphBuilderComplex.graphingBuilder.Len() != 0 || PattyGraphBuilderComplex.matcherBuilder.Len() != 0 {
		t.Fatal("display builders were not reset")
	}
}
