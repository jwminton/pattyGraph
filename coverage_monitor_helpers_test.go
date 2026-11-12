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

func TestCoverageMonitorScalingAndCycles(t *testing.T) {
	resetRuntimeGlobalsForTest()
	empty := &Monitor{}
	if minV, avg, maxV := empty.minAvgMaxHistoryAcrossMatchers(); minV != 0 || avg != 0 || maxV != 0 {
		t.Fatal("empty monitor scaling was nonzero")
	}

	a := NewPredicateMatcher("a")
	a.history = []int{2, 8}
	b := NewPredicateMatcher("b")
	b.history = []int{1, 12}
	PattyGraph = &Monitor{matchers: []MatcherFacade{a, b}}
	botsIndex = 2
	minV, avg, maxV := PattyGraph.minAvgMaxHistoryAcrossMatchers()
	if minV != 1 || avg != 10 || maxV != 12 {
		t.Fatalf("monitor scaling = %d/%.1f/%d", minV, avg, maxV)
	}
	botsIndex = 0
	if minV, avg, maxV := PattyGraph.minAvgMaxHistoryAcrossMatchers(); minV != 0 || avg != 0 || maxV != 0 {
		t.Fatal("monitor without historical rows was nonzero")
	}

	forceZeroStart = true
	currentCycle = 30
	initCycle()
	if currentCycle != 30 {
		t.Fatal("force-zero startup unexpectedly changed cycle")
	}
	forceZeroStart = false
	initCycle()
	if currentCycle < 0 || currentCycle > 58 {
		t.Fatalf("initialized cycle out of range: %d", currentCycle)
	}
	resetCycle()
	if currentCycle != 0 || renderProgressBar() != strings.Repeat("-", progressBarWidth) {
		t.Fatal("cycle reset/progress rendering failed")
	}
	currentCycle = DefaultIntervalSize
	if renderProgressBar() != strings.Repeat("=", progressBarWidth) {
		t.Fatal("full progress bar rendering failed")
	}

	setupMonitorPipelineTestGraph()
	PattyGraph.demo = true
	logicalCycles = 9
	displayFreezeCountdown = 2
	oldView := PattyGraph.secondaryView
	incrementCycle()
	if PattyGraph.secondaryView == oldView || displayFreezeCountdown != 1 || logicalCycles != 10 {
		t.Fatal("demo cycle maintenance failed")
	}
}

func TestCoverageMonitorPathsAndSaveDirectory(t *testing.T) {
	resetRuntimeGlobalsForTest()
	home := t.TempDir()
	t.Setenv("HOME", home)
	if got := expandUser("~/logs"); got != filepath.Join(home, "logs") {
		t.Fatalf("home expansion = %q", got)
	}
	if got := expandUser("plain/path"); got != "plain/path" {
		t.Fatalf("plain path changed: %q", got)
	}

	conf := &MonitorConfig{}
	conf.setSaveDir(filepath.Join(t.TempDir(), "new", "nested"))
	if err := ensureSaveDir(conf); err != nil {
		t.Fatal(err)
	}
	if err := saveDirExists(conf.saveDir); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := saveDirExists(file); err == nil {
		t.Fatal("regular file accepted as save directory")
	}
	if err := saveDirExists(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing save directory accepted")
	}
}

func TestCoverageMonitorSelectionAndMaintenance(t *testing.T) {
	setupMonitorPipelineTestGraph()
	custom := NewPredicateMatcher("custom")
	custom.color = "[orange]"
	custom.markAsAddedMatcher()
	if !placeMatcher(custom, matcherBeforeBots) {
		t.Fatal("failed to place custom matcher")
	}
	PattyGraph.selectedMatcher = custom
	if matcherSelectionColor() != strings.Trim(custom.color, "[]") {
		t.Fatalf("selection color = %q", matcherSelectionColor())
	}
	PattyGraph.deleteSelectedMatcher()
	if matcherNameExists("custom") {
		t.Fatal("selected matcher was not deleted")
	}
	PattyGraph.selectedMatcher = nil
	PattyGraph.deleteSelectedMatcher()
	if isProtectedSystemMatcher(nil) || isProtectedSystemMatcher(PattyGraph.botsMatcher) || !isProtectedSystemMatcher(PattyGraph.linesMatcher) || !isProtectedSystemMatcher(PattyGraph.bytesMatcher) || !isProtectedSystemMatcher(PattyGraph.errsMatcher) || !isProtectedSystemMatcher(PattyGraph.changeMatcher.asMatcher()) {
		t.Fatal("protected matcher classification failed")
	}
	if isProtectedSystemMatcher(NewPredicateMatcher("ordinary")) {
		t.Fatal("ordinary matcher classified as protected")
	}

	PattyGraph.wordsMatcher.peakWords = []string{"word"}
	PattyGraph.wordsMatcher.peakWordsSet["word"] = true
	PattyGraph.changeMatcher.resetState = changeResetReady
	if !PattyGraph.purgeAllPeakContent() || len(PattyGraph.wordsMatcher.peakWords) != 0 || PattyGraph.changeMatcher.resetState == changeResetReady {
		t.Fatal("Peak purge did not clear state")
	}
	if !PattyGraph.purgeAllPeakContent() {
		t.Fatal("Peak purge command did not report handling the request")
	}

	PattyGraph.wordsMatcher.wordFrequency["stale"] = coverageWordStats(1)
	PattyGraph.wordsMatcher.lruTracker.MarkSeen("stale", 1)
	compactCaches()
}

func TestCoverageMonitorFormattingAndColumns(t *testing.T) {
	if sanitize("[red]hello\nworld") != "[red]hello\nworld" {
		t.Fatalf("sanitize mismatch: %q", sanitize("[red]hello\nworld"))
	}
	if truncate("abcdef", 4) != "abc…" || truncate("abc", 4) != "abc" || truncate("abc", 0) != "" {
		t.Fatal("truncate behavior changed")
	}
	if truncateOld("abcdef", 4) == "" || truncateOld("abc", 4) != "abc" {
		t.Fatal("legacy truncate behavior changed")
	}
	lines := []string{"a", "b"}
	if getLineOrBlank(lines, 1) != "b" || getLineOrBlank(lines, 5) != "" {
		t.Fatal("line lookup behavior changed")
	}
	if maxOfFour(1, 9, 3, 4) != 9 || maxOfFour(-1, -2, -3, -4) != -1 {
		t.Fatal("max-of-four behavior changed")
	}

	path := filepath.Join(t.TempDir(), "columns.txt")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteColumnsToFile(f, "one\ntwo", "alpha", "x\ny\nz", "last"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "one") || !strings.Contains(string(data), "last") {
		t.Fatalf("column output mismatch: %q %v", data, err)
	}
}

func TestCoverageMonitorAlertsAndLineErrors(t *testing.T) {
	var nilMonitor *Monitor
	nilMonitor.registerAlertTransition(AlertTransition{})
	nilMonitor.writePendingAlertTransitionsJSONL()
	nilMonitor.clearPendingAlertTransitions()

	setupMonitorPipelineTestGraph()
	transition := AlertTransition{MatcherName: "errs"}
	PattyGraph.registerAlertTransition(transition)
	if len(PattyGraph.pendingAlertTransitions) != 1 {
		t.Fatal("alert transition was not registered")
	}
	generateSidecarJSONL = false
	PattyGraph.writePendingAlertTransitionsJSONL()
	PattyGraph.clearPendingAlertTransitions()
	if len(PattyGraph.pendingAlertTransitions) != 0 {
		t.Fatal("alert transitions were not cleared")
	}

	for status, want := range map[string]bool{"": false, "200": false, "404": true, "500": true} {
		if got := (lineSource{respCode: status}).isError(); got != want {
			t.Fatalf("status %q error=%t, want %t", status, got, want)
		}
	}
}

func TestCoveragePlayConfigFile(t *testing.T) {
	setupMonitorPipelineTestGraph()
	silenceExpectedLogs(t)
	path := filepath.Join(t.TempDir(), "coverage.conf")
	contents := strings.Join([]string{
		"# comment",
		"",
		"!!! push 6",
		"!!! scale 1.2",
	}, "\n")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	PattyGraph.playConfigFile(path)
	if pattyPushFactor != 6 || pattyScaleFactor != 1.2 {
		t.Fatalf("config replay failed: push=%d scale=%.1f", pattyPushFactor, pattyScaleFactor)
	}
	PattyGraph.playConfigFile(filepath.Join(t.TempDir(), "missing.conf"))
}

func TestCoverageMonitorPrePushNil(t *testing.T) {
	PattyGraph = nil
	t.Cleanup(setupMonitorPipelineTestGraph)
	prePush()
	currentCycle = 0
	if got := renderProgressBar(); len(got) != progressBarWidth {
		t.Fatalf("progress bar width = %d", len(got))
	}
	_ = time.Second // retain time import for explicit cycle context
}
