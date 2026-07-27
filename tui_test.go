// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestCtrlPPeakResetWritesOneKeyboardCommand(t *testing.T) {
	setupMonitorPipelineTestGraph()
	path := filepath.Join(t.TempDir(), "peak-reset.jsonl")
	PattyGraph.pattyConfig.jsonFile = path
	generateSidecarJSONL = true
	facts.forced = nil
	setUIHook()
	capture := PattyGraph.app.GetInputCapture()
	if capture == nil {
		t.Fatal("TUI input capture was not installed")
	}

	PattyGraph.selectedMatcher = PattyGraph.linesMatcher
	capture(tcell.NewEventKey(tcell.KeyCtrlP, 0, tcell.ModCtrl))
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("selected matcher purge wrote a model-reset event: %v", err)
	}
	if len(facts.forced) != 0 {
		t.Fatalf("selected matcher purge queued %d reset factoids", len(facts.forced))
	}

	PattyGraph.selectedMatcher = nil
	capture(tcell.NewEventKey(tcell.KeyCtrlP, 0, tcell.ModCtrl))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read keyboard purge event: %v", err)
	}
	text := string(data)
	if !stringsContainAll(text,
		`"event_type":"control_command"`,
		`"source":"keyboard"`,
		`"command":"^p"`,
		`"command_name":"purge"`,
		`"status":"applied"`,
	) {
		t.Fatalf("keyboard purge event = %s", text)
	}
	if strings.Count(text, `"event_type":"control_command"`) != 1 {
		t.Fatalf("keyboard purge wrote duplicate events: %s", text)
	}
	if len(facts.forced) != 1 || facts.forced[0].Name != "model.peakReset" {
		t.Fatalf("keyboard purge factoids = %#v", facts.forced)
	}
}

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

	matcher.detailListingCache = "stale"
	PattyGraph.handleMatcherSelectionGesture(matcher, true)
	if PattyGraph.selectedMatcher != matcher {
		t.Fatal("Ctrl-click did not select matcher")
	}
	if matcher.displayMatchMode != 1 {
		t.Fatalf("displayMatchMode = %d, want 1", matcher.displayMatchMode)
	}
	if matcher.detailListingCache != "" {
		t.Fatal("Ctrl-click did not invalidate matcher detail cache")
	}

	matcher.detailListingCache = "stale"
	PattyGraph.handleMatcherSelectionGesture(matcher, true)
	if PattyGraph.selectedMatcher != matcher {
		t.Fatal("Ctrl-click cleared the selected matcher")
	}
	if matcher.displayMatchMode != 2 {
		t.Fatalf("displayMatchMode = %d, want 2", matcher.displayMatchMode)
	}
	if matcher.detailListingCache != "" {
		t.Fatal("second Ctrl-click did not invalidate matcher detail cache")
	}

	PattyGraph.handleMatcherSelectionGesture(matcher, true)
	if matcher.displayMatchMode != 0 {
		t.Fatalf("displayMatchMode = %d, want wraparound to 0", matcher.displayMatchMode)
	}
}

func TestChangeSelectionDoesNotActivateMatcherSourceLineArea(t *testing.T) {
	setupMonitorPipelineTestGraph()
	PattyGraph.showTicker = false
	baseHeight := len(PattyGraph.matchers) - 1

	change := PattyGraph.changeMatcher
	change.firstMatchLine = "change should not render a source line"
	PattyGraph.setSelectedMatcher(change.asMatcher())
	if PattyGraph.selectedMatcher != change.asMatcher() {
		t.Fatal("change matcher was not selected")
	}
	if change.displaysSelectedSourceLine() {
		t.Fatal("change matcher reports source-line display capability")
	}
	if got := PattyGraph.sparkPanelHeight(); got != baseHeight {
		t.Fatalf("change selection spark height = %d, want %d", got, baseHeight)
	}
	if text := PattyGraph.selectedMatcherSourceLine(); strings.Contains(text, change.firstMatchLine) || text != "" {
		t.Fatalf("change source line unexpectedly rendered: %q", text)
	}

	ordinary := PattyGraph.linesMatcher
	ordinary.firstMatchLine = "ordinary matcher source line"
	PattyGraph.setSelectedMatcher(ordinary)
	if !ordinary.displaysSelectedSourceLine() {
		t.Fatal("ordinary matcher lost source-line display capability")
	}
	if got := PattyGraph.sparkPanelHeight(); got != baseHeight+2 {
		t.Fatalf("ordinary selection spark height = %d, want %d", got, baseHeight+2)
	}
	if text := PattyGraph.selectedMatcherSourceLine(); !strings.Contains(text, ordinary.firstMatchLine) {
		t.Fatalf("ordinary matcher source line was not rendered: %q", text)
	}
}

func TestMatcherAtDetailRowUsesNormalizedPanelRow(t *testing.T) {
	first := NewPredicateMatcher("first")
	first.detailRowCount = 2
	second := NewPredicateMatcher("second")
	third := NewPredicateMatcher("third")
	third.detailRowCount = 1
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
