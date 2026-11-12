// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"strings"
	"testing"
	"time"
)

func TestCoverageFormattingHelpers(t *testing.T) {
	durations := []time.Duration{
		0,
		500 * time.Millisecond,
		5 * time.Second,
		90 * time.Second,
		2*time.Hour + 3*time.Minute,
		49*time.Hour + 5*time.Minute,
	}
	for _, d := range durations {
		if formatUptime(d) == "" || formatShortDuration(d) == "" || formatDuration(d) == "" {
			t.Fatalf("duration formatters returned empty output for %v", d)
		}
	}

	counts := []int{0, 1, 999, 1_000, 10_000, 1_000_000, 1_000_000_000}
	for _, value := range counts {
		if formatCounts(value) == "" || trimmedCounts(value) == "" {
			t.Fatalf("count formatter returned empty output for %d", value)
		}
		if formatCountsUint64(uint64(value)) == "" {
			t.Fatalf("uint count formatter returned empty output for %d", value)
		}
	}

	bytesValues := []int{0, 1, 1023, 1024, 1024 * 1024, 1024 * 1024 * 1024}
	for _, value := range bytesValues {
		if formatBytes(value) == "" || formatBytes64(uint64(value)) == "" {
			t.Fatalf("byte formatter returned empty output for %d", value)
		}
	}
}

func TestCoverageUtilityClassifiersAndDisplayHelpers(t *testing.T) {
	resetRuntimeGlobalsForTest()
	pattyGracePeriod = 4
	for _, depth := range []int{-1, 0, 1, 2, 4, 80, 100} {
		if pattyMonoColorForInt(depth) == "" {
			t.Fatalf("missing mono color for depth %d", depth)
		}
	}

	if !areLikelyIPPatterns([]string{"192.168.1", "10.20.30"}) {
		t.Fatal("expected IP-like patterns")
	}
	if areLikelyIPPatterns([]string{"example.com"}) || areLikelyIPPatterns([]string{"x.1.2"}) {
		t.Fatal("accepted a non-IP-like pattern")
	}
	if !isLowHistory(monoColors[0]) || !isLowishHistory(monoColors[3]) || isLowHistory("white") {
		t.Fatal("history color classification failed")
	}

	if got := sparklineFromArray(0, 10, []int{0, 1, 5, 10}); got == "" {
		t.Fatal("sparkline was empty")
	}
	if got := sparklineFromArray(-10, -1, []int{-10, -5, -1}); got == "" {
		t.Fatal("negative sparkline was empty")
	}
	if got := miniReverseSparklineFromArray([]int{0, 1, 10, 100}); got == "" {
		t.Fatal("mini sparkline was empty")
	}

	for _, value := range []int{0, 1, 15, 16, 31, 32, 63, 64, 127, 128, 255, 256, 1024, 4096} {
		if classifySizeBandKey(value) == "" || classifyTokenBucket(value) == "" {
			t.Fatalf("classification returned empty output for %d", value)
		}
	}
}

func TestCoverageVisibleTextAndStatisticalHelpers(t *testing.T) {
	text := "[red]alpha[default] beta gamma"
	if visibleRuneWidth(text) != len("alpha beta gamma") {
		t.Fatalf("unexpected visible width: %d", visibleRuneWidth(text))
	}
	for _, tc := range []struct {
		offset int
		width  int
	}{
		{0, 5}, {2, 8}, {100, 10}, {0, 0},
	} {
		visible, consumed := sliceFromVisibleOffset(text, tc.offset, tc.width, "[:black]")
		if consumed < 0 || (tc.width > 0 && tc.offset < 20 && visible == "") {
			t.Fatalf("bad visible slice offset=%d width=%d: %q, %d", tc.offset, tc.width, visible, consumed)
		}
	}
	if got := splitFactoidForWidth("[red]alpha beta gamma delta[default]", 8); len(got) < 2 {
		t.Fatalf("expected wrapped factoid, got %#v", got)
	}
	if got := splitFactoidForWidth("alpha", 0); len(got) == 0 {
		t.Fatal("zero-width split returned no content")
	}

	for _, history := range [][]int{nil, {0}, {1, 1, 1}, {0, 10, 0, 10}, {1, 2, 4, 8, 16}} {
		_ = spikiness(history)
		_ = burstiness(history)
	}
	if stripBrackets("[red]value[default]") != "value" {
		t.Fatal("style tags were not stripped")
	}
}

func TestCoverageRingAndAccumulatorMutation(t *testing.T) {
	r := &ringBuffer{}
	for i := 1; i <= 6; i++ {
		r.Push(i)
	}
	r.SetAt(0, 10)
	r.AddAt(0, 5)
	r.unsafeAddAt(1, 3)
	if r.At(0) != 15 || r.Total() == 0 || len(r.FlatSlice()) != r.Len() {
		t.Fatalf("unexpected ring state: %#v", r.FlatSlice())
	}

	a := accumulatorFor(r)
	defer poolAccumulator(a)
	a.unsafeAddAt(0, 2)
	clone := &ringSeriesAccumulator{}
	a.CloneInto(clone)
	clone.MergeFrom(a)
	clone.EnsureCapacityMatch(a)
	if clone.Len() != a.Len() || clone.unsafeAt(0) <= a.unsafeAt(0) {
		t.Fatal("accumulator clone/merge failed")
	}
	if len(clone.Slice()) != clone.Len() || len(clone.ReverseSlice()) != clone.Len() {
		t.Fatal("accumulator slices have the wrong length")
	}
}

func TestCoverageHelpText(t *testing.T) {
	checks := map[string]string{
		printVersion():          PattyGraphVersion,
		usageText():             "Usage:",
		factHelpText():          "Factoids",
		colorHelpText():         "Matcher colors",
		terminalScreenHelp():    "Layout:",
		inlineCommandHelp():     "!!!",
		printBuiltinWordLists(): "Built",
	}
	for text, want := range checks {
		if !strings.Contains(text, want) {
			t.Fatalf("help text missing %q", want)
		}
	}
}
