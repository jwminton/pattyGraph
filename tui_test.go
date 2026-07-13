// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import "testing"

func TestHandleMatcherSelectionGesture(t *testing.T) {
	setupMonitorPipelineTestGraph()
	PattyGraph.showTicker = false
	matcher := NewPredicateMatcher("probe")

	PattyGraph.handleMatcherSelectionGesture(matcher, false)
	if PattyGraph.selectedMatcher != matcher {
		t.Fatal("normal click did not select matcher")
	}

	PattyGraph.handleMatcherSelectionGesture(matcher, false)
	if PattyGraph.selectedMatcher != nil {
		t.Fatal("second normal click did not clear matcher selection")
	}

	matcher.displayMatchedCache = "stale"
	PattyGraph.handleMatcherSelectionGesture(matcher, true)
	if PattyGraph.selectedMatcher != matcher {
		t.Fatal("Ctrl-click did not select matcher")
	}
	if matcher.displayMatchMode != 1 {
		t.Fatalf("displayMatchMode = %d, want 1", matcher.displayMatchMode)
	}
	if matcher.displayMatchedCache != "" {
		t.Fatal("Ctrl-click did not invalidate matcher detail cache")
	}

	matcher.displayMatchedCache = "stale"
	PattyGraph.handleMatcherSelectionGesture(matcher, true)
	if PattyGraph.selectedMatcher != matcher {
		t.Fatal("Ctrl-click cleared the selected matcher")
	}
	if matcher.displayMatchMode != 2 {
		t.Fatalf("displayMatchMode = %d, want 2", matcher.displayMatchMode)
	}
	if matcher.displayMatchedCache != "" {
		t.Fatal("second Ctrl-click did not invalidate matcher detail cache")
	}

	PattyGraph.handleMatcherSelectionGesture(matcher, true)
	if matcher.displayMatchMode != 0 {
		t.Fatalf("displayMatchMode = %d, want wraparound to 0", matcher.displayMatchMode)
	}
}

func TestMatcherAtDetailRowUsesNormalizedPanelRow(t *testing.T) {
	first := NewPredicateMatcher("first")
	first.matchedDisplayCount = 2
	second := NewPredicateMatcher("second")
	third := NewPredicateMatcher("third")
	third.matchedDisplayCount = 1
	interesting := NewInterestingWordMatcher("words", 60)

	monitor := &Monitor{
		matchers: []MatcherFacade{first, second, third, interesting},
	}
	tests := []struct {
		normalizedRow int
		want          *Matcher
	}{
		{normalizedRow: -1, want: nil},
		{normalizedRow: 0, want: first},
		{normalizedRow: 2, want: first},
		{normalizedRow: 3, want: second},
		{normalizedRow: 4, want: third},
		{normalizedRow: 5, want: third},
		{normalizedRow: 6, want: nil},
	}

	for _, test := range tests {
		if got := monitor.matcherAtDetailRow(test.normalizedRow); got != test.want {
			gotName := "<nil>"
			if got != nil {
				gotName = got.name
			}
			wantName := "<nil>"
			if test.want != nil {
				wantName = test.want.name
			}
			t.Errorf("normalized row %d resolved to %s, want %s", test.normalizedRow, gotName, wantName)
		}
	}
}
