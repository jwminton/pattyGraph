// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func matcherIndexByNameForTest(name string) int {
	for i, matcher := range PattyGraph.matchers {
		if matcher.matcherName() == name {
			return i
		}
	}
	return -1
}

func matcherCountByNameForTest(name string) int {
	count := 0
	for _, matcher := range PattyGraph.matchers {
		if matcher.matcherName() == name {
			count++
		}
	}
	return count
}

func TestInlineAddUndecoratedNameInsertsBeforeBots(t *testing.T) {
	setupMonitorPipelineTestGraph()

	invokeInlineCommand("!!! add googlebot")

	googlebotIndex := matcherIndexByNameForTest("googlebot")
	if googlebotIndex == -1 {
		t.Fatal("googlebot matcher was not added")
	}
	if googlebotIndex != botsIndex-1 {
		t.Fatalf("googlebot index = %d, want immediately before Bots at %d", googlebotIndex, botsIndex-1)
	}
}

func TestInlineAddPlusNameInsertsFirst(t *testing.T) {
	setupMonitorPipelineTestGraph()

	invokeInlineCommand("!!! add +googlebot")

	if got := PattyGraph.matchers[0].matcherName(); got != "googlebot" {
		t.Fatalf("first matcher = %q, want googlebot", got)
	}
}

func TestInlineAddMinusNameInsertsBeforeBots(t *testing.T) {
	setupMonitorPipelineTestGraph()

	invokeInlineCommand("!!! add -googlebot")

	googlebotIndex := matcherIndexByNameForTest("googlebot")
	if googlebotIndex == -1 {
		t.Fatal("googlebot matcher was not added")
	}
	if googlebotIndex != botsIndex-1 {
		t.Fatalf("googlebot index = %d, want immediately before Bots at %d", googlebotIndex, botsIndex-1)
	}
}

func TestInlineAddStarNameInsertsBelowBotsBeforeLines(t *testing.T) {
	setupMonitorPipelineTestGraph()

	invokeInlineCommand("!!! add *googlebot")

	googlebotIndex := matcherIndexByNameForTest("googlebot")
	linesIndex := matcherIndexByNameForTest("lines")
	if googlebotIndex == -1 {
		t.Fatal("googlebot matcher was not added")
	}
	if googlebotIndex != botsIndex+1 {
		t.Fatalf("googlebot index = %d, want immediately after Bots at %d", googlebotIndex, botsIndex+1)
	}
	if googlebotIndex != linesIndex-1 {
		t.Fatalf("googlebot index = %d, want immediately before lines at %d", googlebotIndex, linesIndex-1)
	}
}

func TestInlineAddRejectsDuplicateMatcherName(t *testing.T) {
	setupMonitorPipelineTestGraph()

	first := invokeInlineCommand("!!! add googlebot")
	if first.Status != InlineCommandStatusApplied {
		t.Fatalf("first status = %q, want applied: %#v", first.Status, first.Result)
	}

	duplicate := invokeInlineCommand("!!! add googlebot")
	if duplicate.Status != InlineCommandStatusRejected {
		t.Fatalf("duplicate status = %q, want rejected: %#v", duplicate.Status, duplicate.Result)
	}
	if duplicate.Result["error"] != "duplicate matcher name" {
		t.Fatalf("duplicate error = %v, want duplicate matcher name", duplicate.Result["error"])
	}
	if got := matcherCountByNameForTest("googlebot"); got != 1 {
		t.Fatalf("googlebot matcher count = %d, want 1", got)
	}
}

func TestInlineJSONFileImpliesJSONOutput(t *testing.T) {
	setupMonitorPipelineTestGraph()
	generateSidecarJSONL = false
	PattyGraph.pattyConfig.setSaveDir("/tmp/patty")

	result := invokeInlineCommand("!!! json-file current.jsonl")

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want applied: %#v", result.Status, result.Result)
	}
	if !generateSidecarJSONL {
		t.Fatal("generateSidecarJSONL = false, want true")
	}
	if got, want := PattyGraph.pattyConfig.jsonFile, "/tmp/patty/current.jsonl"; got != want {
		t.Fatalf("jsonFile = %q, want %q", got, want)
	}
}

func TestInlineJSONTogglesSidecarOutput(t *testing.T) {
	setupMonitorPipelineTestGraph()
	generateSidecarJSONL = false

	invokeInlineCommand("!!! json on")
	if !generateSidecarJSONL {
		t.Fatal("json on did not enable generateSidecarJSONL")
	}

	invokeInlineCommand("!!! json off")
	if generateSidecarJSONL {
		t.Fatal("json off did not disable generateSidecarJSONL")
	}

	invokeInlineCommand("!!! json-file current.jsonl")
	invokeInlineCommand("!!! json off")
	if !generateSidecarJSONL {
		t.Fatal("json off disabled output implied by json-file")
	}
}

func TestInlineAddRejectsDuplicateMatcherNameAcrossPlacementPrefixes(t *testing.T) {
	setupMonitorPipelineTestGraph()

	first := invokeInlineCommand("!!! add +googlebot")
	if first.Status != InlineCommandStatusApplied {
		t.Fatalf("first status = %q, want applied: %#v", first.Status, first.Result)
	}

	duplicate := invokeInlineCommand("!!! add *googlebot")
	if duplicate.Status != InlineCommandStatusRejected {
		t.Fatalf("duplicate status = %q, want rejected: %#v", duplicate.Status, duplicate.Result)
	}
	if got := matcherCountByNameForTest("googlebot"); got != 1 {
		t.Fatalf("googlebot matcher count = %d, want 1", got)
	}
}

func TestInlineAddMatcherNamesAreCaseSensitive(t *testing.T) {
	setupMonitorPipelineTestGraph()

	lower := invokeInlineCommand("!!! add googlebot")
	if lower.Status != InlineCommandStatusApplied {
		t.Fatalf("lower status = %q, want applied: %#v", lower.Status, lower.Result)
	}
	upper := invokeInlineCommand("!!! add Googlebot")
	if upper.Status != InlineCommandStatusApplied {
		t.Fatalf("upper status = %q, want applied: %#v", upper.Status, upper.Result)
	}

	if got := matcherCountByNameForTest("googlebot"); got != 1 {
		t.Fatalf("googlebot matcher count = %d, want 1", got)
	}
	if got := matcherCountByNameForTest("Googlebot"); got != 1 {
		t.Fatalf("Googlebot matcher count = %d, want 1", got)
	}
}

func TestInlineDelUndecoratedNameRemovesMatcher(t *testing.T) {
	setupMonitorPipelineTestGraph()
	invokeInlineCommand("!!! add googlebot")

	invokeInlineCommand("!!! del googlebot")

	if matcherIndexByNameForTest("googlebot") != -1 {
		t.Fatal("googlebot matcher still exists after undecorated del")
	}
}

func TestInlineDelDecoratedNameRemovesMatcher(t *testing.T) {
	setupMonitorPipelineTestGraph()
	invokeInlineCommand("!!! add googlebot")

	invokeInlineCommand("!!! del *googlebot")

	if matcherIndexByNameForTest("googlebot") != -1 {
		t.Fatal("googlebot matcher still exists after decorated del")
	}
}

func TestInlineSelectInterestingMatcherByKey(t *testing.T) {
	setupMonitorPipelineTestGraph()
	refs := PattyGraph.refsMatcher
	refs.currentListing = []string{"www.zanbil.ir", "Suspicious", "filter"}
	PattyGraph.selectedInterestingMatcher = nil

	result := invokeInlineCommand("!!! select --refs Suspicious")

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q", result.Status, InlineCommandStatusApplied)
	}
	if result.Result["action"] != "select_interesting" {
		t.Fatalf("action = %v, want select_interesting", result.Result["action"])
	}
	if result.Result["matcher"] != "refs" {
		t.Fatalf("matcher = %v, want refs", result.Result["matcher"])
	}
	if result.Result["selection"] != "Suspicious" {
		t.Fatalf("selection = %v, want Suspicious", result.Result["selection"])
	}
	if result.Result["selection_index"] != 1 {
		t.Fatalf("selection_index = %v, want 1", result.Result["selection_index"])
	}
	if PattyGraph.selectedInterestingMatcher != refs {
		t.Fatal("refs matcher was not selected")
	}
	if refs.selectedKey != "Suspicious" {
		t.Fatalf("selectedKey = %q, want Suspicious", refs.selectedKey)
	}
}

func TestInlineSelectWithoutArgsClearsInterestingSelection(t *testing.T) {
	setupMonitorPipelineTestGraph()
	refs := PattyGraph.refsMatcher
	refs.currentListing = []string{"www.zanbil.ir", "Suspicious", "filter"}
	refs.selectDisplayItem(1)
	if PattyGraph.selectedInterestingMatcher != refs {
		t.Fatal("refs matcher was not selected before clear")
	}

	result := invokeInlineCommand("!!! select")

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q", result.Status, InlineCommandStatusApplied)
	}
	if result.Result["action"] != "clear_selection" {
		t.Fatalf("action = %v, want clear_selection", result.Result["action"])
	}
	if PattyGraph.selectedInterestingMatcher != nil {
		t.Fatal("selected interesting matcher was not cleared")
	}
	if refs.selectedKey != "" {
		t.Fatalf("selectedKey = %q, want empty", refs.selectedKey)
	}
}

func TestInlineSelectIsCaseSensitive(t *testing.T) {
	setupMonitorPipelineTestGraph()
	refs := PattyGraph.refsMatcher
	refs.currentListing = []string{"Filter", "filter"}
	PattyGraph.selectedInterestingMatcher = nil

	upper := invokeInlineCommand("!!! select --refs Filter")
	if upper.Status != InlineCommandStatusApplied {
		t.Fatalf("upper status = %q, want %q", upper.Status, InlineCommandStatusApplied)
	}
	if upper.Result["selection"] != "Filter" {
		t.Fatalf("upper selection = %v, want Filter", upper.Result["selection"])
	}
	if refs.selectedKey != "Filter" {
		t.Fatalf("selectedKey = %q, want Filter", refs.selectedKey)
	}

	lower := invokeInlineCommand("!!! select --refs filter")
	if lower.Status != InlineCommandStatusApplied {
		t.Fatalf("lower status = %q, want %q", lower.Status, InlineCommandStatusApplied)
	}
	if lower.Result["selection"] != "filter" {
		t.Fatalf("lower selection = %v, want filter", lower.Result["selection"])
	}
	if refs.selectedKey != "filter" {
		t.Fatalf("selectedKey = %q, want filter", refs.selectedKey)
	}
}

func TestInlineControlCommandTogglesControlFileProcessing(t *testing.T) {
	setupMonitorPipelineTestGraph()
	enableControlFile = false

	invokeInlineCommand("!!! control")
	if !enableControlFile {
		t.Fatal("enableControlFile = false after !!! control, want true")
	}

	invokeInlineCommand("!!! control off")
	if enableControlFile {
		t.Fatal("enableControlFile = true after !!! control off, want false")
	}

	invokeInlineCommand("!!! control on")
	if !enableControlFile {
		t.Fatal("enableControlFile = false after !!! control on, want true")
	}
}

func TestInlineControlFileRuntimeUpdatesConfigOnly(t *testing.T) {
	setupMonitorPipelineTestGraph()
	enableControlFile = false
	PattyGraph.pattyConfig.setSaveDir("/tmp/patty")

	result := invokeInlineCommand("!!! control-file current.control")

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q", result.Status, InlineCommandStatusApplied)
	}
	if enableControlFile {
		t.Fatal("enableControlFile = true after runtime control-file, want unchanged false")
	}
	if got, want := PattyGraph.pattyConfig.controlFile, "/tmp/patty/current.control"; got != want {
		t.Fatalf("controlFile = %q, want %q", got, want)
	}
	if result.Result["runtime_effect"] != "next_config_only" {
		t.Fatalf("runtime_effect = %v, want next_config_only", result.Result["runtime_effect"])
	}
}

func TestConfigInlineControlFileImpliesControl(t *testing.T) {
	setupMonitorPipelineTestGraph()
	enableControlFile = false
	PattyGraph.pattyConfig.setSaveDir("/tmp/patty")

	result := invokeInlineCommandWithOptions(
		"!!! control-file current.control",
		InlineCommandOptions{allowCreateSaveDir: true},
	)

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q", result.Status, InlineCommandStatusApplied)
	}
	if !enableControlFile {
		t.Fatal("enableControlFile = false after config control-file, want true")
	}
	if got, want := PattyGraph.pattyConfig.controlFile, "/tmp/patty/current.control"; got != want {
		t.Fatalf("controlFile = %q, want %q", got, want)
	}
}

func TestInlineSaveDirRejectsMissingDirectoryResult(t *testing.T) {
	setupMonitorPipelineTestGraph()
	existing := t.TempDir()
	missing := filepath.Join(t.TempDir(), "missing")
	PattyGraph.pattyConfig.setSaveDir(existing)

	result := invokeInlineCommand("!!! save-dir " + missing)

	if result.Status != InlineCommandStatusRejected {
		t.Fatalf("status = %q, want %q", result.Status, InlineCommandStatusRejected)
	}
	if result.Result["name"] != "save-dir" {
		t.Fatalf("result name = %v, want save-dir", result.Result["name"])
	}
	if result.Result["path"] != missing {
		t.Fatalf("result path = %v, want %s", result.Result["path"], missing)
	}
	if got := PattyGraph.pattyConfig.saveDir; got != existing {
		t.Fatalf("saveDir = %q, want unchanged existing dir %q", got, existing)
	}
}

func TestConfigInlineSaveDirCanCreateMissingDirectory(t *testing.T) {
	setupMonitorPipelineTestGraph()
	missing := filepath.Join(t.TempDir(), "missing", "nested")

	result := invokeInlineCommandWithOptions(
		"!!! save-dir "+missing,
		InlineCommandOptions{allowCreateSaveDir: true},
	)

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q", result.Status, InlineCommandStatusApplied)
	}
	if got := PattyGraph.pattyConfig.saveDir; got != missing {
		t.Fatalf("saveDir = %q, want %q", got, missing)
	}
	if info, err := os.Stat(missing); err != nil {
		t.Fatalf("config save-dir was not created: %v", err)
	} else if !info.IsDir() {
		t.Fatalf("config save-dir path is not a directory")
	}
}

func TestInlineAlertSetShowAndRangeValidation(t *testing.T) {
	setupMonitorPipelineTestGraph()

	above := invokeInlineCommand("!!! alert errs above 50")
	if above.Status != InlineCommandStatusApplied {
		t.Fatalf("above status = %q, want applied: %#v", above.Status, above.Result)
	}
	if PattyGraph.errsMatcher.AlertAbove.Threshold != 50 {
		t.Fatalf("above threshold = %d, want 50", PattyGraph.errsMatcher.AlertAbove.Threshold)
	}

	below := invokeInlineCommand("!!! alert errs below 10")
	if below.Status != InlineCommandStatusApplied {
		t.Fatalf("below status = %q, want applied: %#v", below.Status, below.Result)
	}
	if PattyGraph.errsMatcher.AlertBelow.Threshold != 10 {
		t.Fatalf("below threshold = %d, want 10", PattyGraph.errsMatcher.AlertBelow.Threshold)
	}

	show := invokeInlineCommand("!!! alert errs")
	if show.Status != InlineCommandStatusApplied {
		t.Fatalf("show status = %q, want applied", show.Status)
	}
	if show.Result["action"] != "show_alert" {
		t.Fatalf("show action = %v, want show_alert", show.Result["action"])
	}
	if show.Result["matcher"] != "errs" {
		t.Fatalf("show matcher = %v, want errs", show.Result["matcher"])
	}

	overlap := invokeInlineCommand("!!! alert errs below 100")
	if overlap.Status != InlineCommandStatusRejected {
		t.Fatalf("overlap status = %q, want rejected", overlap.Status)
	}
	belowZero := invokeInlineCommand("!!! alert errs below 0")
	if belowZero.Status != InlineCommandStatusRejected {
		t.Fatalf("below zero status = %q, want rejected", belowZero.Status)
	}
}

func TestMatcherAlertsTriggerRecoverAndListOnConsecutiveFluxDepth(t *testing.T) {
	setupMonitorPipelineTestGraph()
	fluxDepth = 2
	invokeInlineCommand("!!! alert errs above 3")
	errs := PattyGraph.errsMatcher

	errs.intervalCount = 3
	errs.push()
	if len(PattyGraph.pendingAlertTransitions) != 0 {
		t.Fatalf("pending transitions after first hit = %d, want 0", len(PattyGraph.pendingAlertTransitions))
	}

	errs.intervalCount = 4
	errs.push()
	if len(PattyGraph.pendingAlertTransitions) != 1 {
		t.Fatalf("pending transitions after second hit = %d, want 1", len(PattyGraph.pendingAlertTransitions))
	}
	trigger := PattyGraph.pendingAlertTransitions[0]
	if trigger.Status != AlertStatusTriggered || trigger.Direction != AlertDirectionAbove || trigger.Value != 4 || trigger.Threshold != 3 {
		t.Fatalf("trigger transition = %#v", trigger)
	}

	alerts := invokeInlineCommand("!!! alerts")
	active, ok := alerts.Result["active_alerts"].([]map[string]interface{})
	if !ok || len(active) != 1 {
		t.Fatalf("active alerts = %#v, want one alert", alerts.Result["active_alerts"])
	}
	if active[0]["matcher"] != "errs" || active[0]["direction"] != AlertDirectionAbove {
		t.Fatalf("active alert = %#v", active[0])
	}

	PattyGraph.clearPendingAlertTransitions()
	errs.intervalCount = 10
	errs.push()
	if len(PattyGraph.pendingAlertTransitions) != 0 {
		t.Fatalf("pending transitions while still active = %d, want 0", len(PattyGraph.pendingAlertTransitions))
	}

	errs.intervalCount = 2
	errs.push()
	if len(PattyGraph.pendingAlertTransitions) != 0 {
		t.Fatalf("pending transitions after first clear = %d, want 0", len(PattyGraph.pendingAlertTransitions))
	}
	errs.intervalCount = 0
	errs.push()
	if len(PattyGraph.pendingAlertTransitions) != 1 {
		t.Fatalf("pending transitions after second clear = %d, want 1", len(PattyGraph.pendingAlertTransitions))
	}
	recovered := PattyGraph.pendingAlertTransitions[0]
	if recovered.Status != AlertStatusRecovered || recovered.Direction != AlertDirectionAbove || recovered.Value != 0 {
		t.Fatalf("recovered transition = %#v", recovered)
	}
}

func TestInlineAlertClearActiveBoundDoesNotEmitRecovered(t *testing.T) {
	setupMonitorPipelineTestGraph()
	fluxDepth = 1
	invokeInlineCommand("!!! alert errs below 1")
	errs := PattyGraph.errsMatcher
	errs.intervalCount = 0
	errs.push()
	if len(PattyGraph.pendingAlertTransitions) != 1 {
		t.Fatalf("pending transitions after trigger = %d, want 1", len(PattyGraph.pendingAlertTransitions))
	}
	PattyGraph.clearPendingAlertTransitions()

	clearResult := invokeInlineCommand("!!! alert errs clear below")
	if clearResult.Status != InlineCommandStatusApplied {
		t.Fatalf("clear status = %q, want applied", clearResult.Status)
	}
	if clearResult.Result["was_active"] != true {
		t.Fatalf("was_active = %v, want true", clearResult.Result["was_active"])
	}
	if PattyGraph.errsMatcher.AlertBelow.Enabled || PattyGraph.errsMatcher.AlertBelow.Active {
		t.Fatalf("below bound after clear = %#v", PattyGraph.errsMatcher.AlertBelow)
	}
	if len(PattyGraph.pendingAlertTransitions) != 0 {
		t.Fatalf("pending transitions after manual clear = %d, want 0", len(PattyGraph.pendingAlertTransitions))
	}
}

func TestFluxChangeResetsAlertRuntimeStateButKeepsBounds(t *testing.T) {
	setupMonitorPipelineTestGraph()
	fluxDepth = 1
	invokeInlineCommand("!!! alert errs above 1")
	errs := PattyGraph.errsMatcher
	errs.intervalCount = 2
	errs.push()
	if !errs.AlertAbove.Active {
		t.Fatal("above alert active = false, want true before flux change")
	}
	if len(PattyGraph.pendingAlertTransitions) != 1 {
		t.Fatalf("pending transitions = %d, want 1 before flux change", len(PattyGraph.pendingAlertTransitions))
	}

	if !setFlux(3) {
		t.Fatal("setFlux(3) = false, want true")
	}
	if !errs.AlertAbove.Enabled || errs.AlertAbove.Threshold != 1 {
		t.Fatalf("above bound after flux change = %#v, want enabled threshold 1", errs.AlertAbove)
	}
	if errs.AlertAbove.Active || errs.AlertAbove.HitRun != 0 || errs.AlertAbove.ClearRun != 0 || errs.AlertAbove.LastValue != 0 {
		t.Fatalf("above runtime state after flux change = %#v, want reset", errs.AlertAbove)
	}
	if len(PattyGraph.pendingAlertTransitions) != 0 {
		t.Fatalf("pending transitions after flux change = %d, want 0", len(PattyGraph.pendingAlertTransitions))
	}
}

func TestInlineAddScopedIPReturnsResult(t *testing.T) {
	setupMonitorPipelineTestGraph()

	result := invokeInlineCommand("!!! add ip-91 --ips 91.99.72.15")

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q", result.Status, InlineCommandStatusApplied)
	}
	if result.Result["action"] != "add_matcher" {
		t.Fatalf("action = %v, want add_matcher", result.Result["action"])
	}
	if result.Result["matcher_name"] != "ip-91" {
		t.Fatalf("matcher_name = %v, want ip-91", result.Result["matcher_name"])
	}
	if result.Result["scope"] != "ips" {
		t.Fatalf("scope = %v, want ips", result.Result["scope"])
	}
	patterns, ok := result.Result["patterns"].([]string)
	if !ok || len(patterns) != 1 || patterns[0] != "91.99.72.15" {
		t.Fatalf("patterns = %#v, want [91.99.72.15]", result.Result["patterns"])
	}
}

func TestInlineBadAddReturnsRejectedResult(t *testing.T) {
	setupMonitorPipelineTestGraph()

	result := invokeInlineCommand("!!! add")

	if result.Status != InlineCommandStatusRejected {
		t.Fatalf("status = %q, want %q", result.Status, InlineCommandStatusRejected)
	}
	if result.Result["action"] != "add_matcher" {
		t.Fatalf("action = %v, want add_matcher", result.Result["action"])
	}
	if result.Result["error"] == "" {
		t.Fatal("error was empty")
	}
}
