// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"os"
	"path/filepath"
	"strings"
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
	if generateSidecarJSONL {
		t.Fatal("json off did not disable output with json-file configured")
	}
}

func TestInlineJSONRejectsInvalidBooleanValue(t *testing.T) {
	setupMonitorPipelineTestGraph()
	generateSidecarJSONL = false

	result := invokeInlineCommand("!!! json maybe")

	if result.Status != InlineCommandStatusRejected {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusRejected, result.Result)
	}
	if result.Result["error"] != "invalid boolean value" {
		t.Fatalf("error = %v, want invalid boolean value", result.Result["error"])
	}
	if generateSidecarJSONL {
		t.Fatal("json maybe enabled JSONL output")
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

func TestInlineDelRejectsMissingMatcher(t *testing.T) {
	setupMonitorPipelineTestGraph()

	result := invokeInlineCommand("!!! del noSuchMatcher")

	if result.Status != InlineCommandStatusRejected {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusRejected, result.Result)
	}
	if result.Result["error"] != "matcher not found" {
		t.Fatalf("error = %v, want matcher not found", result.Result["error"])
	}
	if result.Result["matcher_name"] != "noSuchMatcher" {
		t.Fatalf("matcher_name = %v, want noSuchMatcher", result.Result["matcher_name"])
	}
}

func TestInlineDelRejectsProtectedSystemMatcher(t *testing.T) {
	setupMonitorPipelineTestGraph()

	result := invokeInlineCommand("!!! del lines")

	if result.Status != InlineCommandStatusRejected {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusRejected, result.Result)
	}
	if result.Result["error"] != "matcher cannot be deleted" {
		t.Fatalf("error = %v, want matcher cannot be deleted", result.Result["error"])
	}
	if matcherIndexByNameForTest("lines") == -1 {
		t.Fatal("lines matcher was removed")
	}
}

func TestInlineAddBotsEnablesAutoAdd(t *testing.T) {
	setupMonitorPipelineTestGraph()
	PattyGraph.botsMatcher.disableAutoAdd = true

	result := invokeInlineCommand("!!! add Bots")

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusApplied, result.Result)
	}
	if result.Result["action"] != "enable_bots_auto_add" {
		t.Fatalf("action = %v, want enable_bots_auto_add", result.Result["action"])
	}
	if matcherIndexByNameForTest(BotsMatcherName) == -1 {
		t.Fatal("Bots matcher was removed")
	}
	if matcherCountByNameForTest(BotsMatcherName) != 1 {
		t.Fatalf("Bots matcher count = %d, want 1", matcherCountByNameForTest(BotsMatcherName))
	}
	if PattyGraph.botsMatcher.disableAutoAdd {
		t.Fatal("Bots disableAutoAdd = true, want false")
	}
}

func TestInlineDelBotsDisablesAutoAdd(t *testing.T) {
	setupMonitorPipelineTestGraph()
	PattyGraph.botsMatcher.disableAutoAdd = false

	result := invokeInlineCommand("!!! del Bots")

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusApplied, result.Result)
	}
	if result.Result["action"] != "disable_bots_auto_add" {
		t.Fatalf("action = %v, want disable_bots_auto_add", result.Result["action"])
	}
	if matcherIndexByNameForTest(BotsMatcherName) == -1 {
		t.Fatal("Bots matcher was removed")
	}
	if !PattyGraph.botsMatcher.disableAutoAdd {
		t.Fatal("Bots disableAutoAdd = false, want true")
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

func TestInlineControlCommandAcceptsBooleanAliases(t *testing.T) {
	setupMonitorPipelineTestGraph()
	enableControlFile = false

	result := invokeInlineCommand("!!! control yes")
	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusApplied, result.Result)
	}
	if !enableControlFile {
		t.Fatal("control yes did not enable control file")
	}

	result = invokeInlineCommand("!!! control no")
	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusApplied, result.Result)
	}
	if enableControlFile {
		t.Fatal("control no did not disable control file")
	}
}

func TestInlineControlCommandRejectsInvalidBooleanValue(t *testing.T) {
	setupMonitorPipelineTestGraph()
	enableControlFile = false

	result := invokeInlineCommand("!!! control maybe")

	if result.Status != InlineCommandStatusRejected {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusRejected, result.Result)
	}
	if result.Result["error"] != "invalid boolean value" {
		t.Fatalf("error = %v, want invalid boolean value", result.Result["error"])
	}
	if enableControlFile {
		t.Fatal("control maybe enabled control file")
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

func TestInlineAddAcceptsFlagFirstScope(t *testing.T) {
	setupMonitorPipelineTestGraph()

	result := invokeInlineCommand("!!! add --refs refProbe filter")

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusApplied, result.Result)
	}
	if result.Result["matcher_name"] != "refProbe" {
		t.Fatalf("matcher_name = %v, want refProbe", result.Result["matcher_name"])
	}
	if result.Result["scope"] != "refs" {
		t.Fatalf("scope = %v, want refs", result.Result["scope"])
	}
	if matcherIndexByNameForTest("-refs") != -1 {
		t.Fatal("flag-first add created malformed -refs matcher")
	}
	if matcherIndexByNameForTest("refProbe") == -1 {
		t.Fatal("refProbe matcher was not added")
	}
	patterns, ok := result.Result["patterns"].([]string)
	if !ok || len(patterns) != 1 || patterns[0] != "filter" {
		t.Fatalf("patterns = %#v, want [filter]", result.Result["patterns"])
	}
}

func TestInlineAddAcceptsFlagFirstScopeWithPlacement(t *testing.T) {
	setupMonitorPipelineTestGraph()

	result := invokeInlineCommand("!!! add --refs *observerProbe filter")

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusApplied, result.Result)
	}
	if result.Result["matcher_name"] != "observerProbe" {
		t.Fatalf("matcher_name = %v, want observerProbe", result.Result["matcher_name"])
	}
	if result.Result["placement"] != "before_lines" {
		t.Fatalf("placement = %v, want before_lines", result.Result["placement"])
	}
	observerIndex := matcherIndexByNameForTest("observerProbe")
	linesIndex := matcherIndexByNameForTest("lines")
	if observerIndex == -1 {
		t.Fatal("observerProbe matcher was not added")
	}
	if observerIndex <= botsIndex || observerIndex >= linesIndex {
		t.Fatalf("observerProbe index = %d, want below Bots and before lines", observerIndex)
	}
}

func TestInlineAddRejectsScopeFlagWithoutMatcherName(t *testing.T) {
	setupMonitorPipelineTestGraph()

	result := invokeInlineCommand("!!! add --words")

	if result.Status != InlineCommandStatusRejected {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusRejected, result.Result)
	}
	if result.Result["error"] != "missing matcher name" {
		t.Fatalf("error = %v, want missing matcher name", result.Result["error"])
	}
}

func TestInlineAddRejectsFlagShapedMatcherName(t *testing.T) {
	setupMonitorPipelineTestGraph()

	result := invokeInlineCommand("!!! add --unknown pattern")

	if result.Status != InlineCommandStatusRejected {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusRejected, result.Result)
	}
	if result.Result["error"] != "matcher name cannot look like a flag" {
		t.Fatalf("error = %v, want matcher name cannot look like a flag", result.Result["error"])
	}
	if result.Result["raw_matcher_name"] != "--unknown" {
		t.Fatalf("raw_matcher_name = %v, want --unknown", result.Result["raw_matcher_name"])
	}
}

func TestInlineAddRejectsMatcherNameWithLiteralPlacementPrefix(t *testing.T) {
	setupMonitorPipelineTestGraph()

	for _, command := range []string{
		"!!! add ++badName pattern",
		"!!! add *+badName pattern",
		"!!! add -+badName pattern",
	} {
		result := invokeInlineCommand(command)
		if result.Status != InlineCommandStatusRejected {
			t.Fatalf("%s status = %q, want %q: %#v", command, result.Status, InlineCommandStatusRejected, result.Result)
		}
		if result.Result["error"] != "matcher name cannot begin with a placement prefix" {
			t.Fatalf("%s error = %v, want matcher name cannot begin with a placement prefix", command, result.Result["error"])
		}
		if result.Result["raw_matcher_name"] == "" {
			t.Fatalf("%s raw_matcher_name was empty", command)
		}
		if result.Result["normalized_matcher_name"] == "" {
			t.Fatalf("%s normalized_matcher_name was empty", command)
		}
	}
}

func TestInlineAddRejectsEmptyNameAfterPlacementPrefix(t *testing.T) {
	setupMonitorPipelineTestGraph()

	result := invokeInlineCommand("!!! add + pattern")

	if result.Status != InlineCommandStatusRejected {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusRejected, result.Result)
	}
	if result.Result["error"] != "matcher name is empty after placement prefix" {
		t.Fatalf("error = %v, want matcher name is empty after placement prefix", result.Result["error"])
	}
	if result.Result["raw_matcher_name"] != "+" {
		t.Fatalf("raw_matcher_name = %v, want +", result.Result["raw_matcher_name"])
	}
}

func TestInlineAddAcceptsFlagFirstRegex(t *testing.T) {
	setupMonitorPipelineTestGraph()

	result := invokeInlineCommand("!!! add --regex botProbe Googlebot|bingbot")

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusApplied, result.Result)
	}
	if result.Result["matcher_name"] != "botProbe" {
		t.Fatalf("matcher_name = %v, want botProbe", result.Result["matcher_name"])
	}
	if result.Result["regex"] != true {
		t.Fatalf("regex = %v, want true", result.Result["regex"])
	}
	if result.Result["scope"] != "regex" {
		t.Fatalf("scope = %v, want regex", result.Result["scope"])
	}
}

func TestInlineFactRejectsUnknownFactName(t *testing.T) {
	setupMonitorPipelineTestGraph()
	facts = NewFactoidGenerator()
	facts.forced = nil

	result := invokeInlineCommand("!!! fact no.such.fact")

	if result.Status != InlineCommandStatusRejected {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusRejected, result.Result)
	}
	if result.Result["error"] != "fact not found" {
		t.Fatalf("error = %v, want fact not found", result.Result["error"])
	}
	if result.Result["fact"] != "no.such.fact" {
		t.Fatalf("fact = %v, want no.such.fact", result.Result["fact"])
	}
	msg, _, _ := facts.Next()
	if !strings.Contains(msg, "Factoid not found: no.such.fact") {
		t.Fatalf("forced factoid = %q, want missing factoid error", msg)
	}
}

func TestInlineFactAcceptsKnownFactName(t *testing.T) {
	setupMonitorPipelineTestGraph()
	facts = NewFactoidGenerator()

	result := invokeInlineCommand("!!! fact output.health")

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusApplied, result.Result)
	}
	if result.Result["fact"] != "output.health" {
		t.Fatalf("fact = %v, want output.health", result.Result["fact"])
	}
}

func TestInlineModeRejectsInvalidModeRange(t *testing.T) {
	setupMonitorPipelineTestGraph()
	invokeInlineCommand("!!! add modeProbe")

	result := invokeInlineCommand("!!! mode modeProbe 9")

	if result.Status != InlineCommandStatusRejected {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusRejected, result.Result)
	}
	if result.Result["error"] != "invalid mode" {
		t.Fatalf("error = %v, want invalid mode", result.Result["error"])
	}
	if result.Result["matcher_name"] != "modeProbe" {
		t.Fatalf("matcher_name = %v, want modeProbe", result.Result["matcher_name"])
	}
	if result.Result["mode"] != 9 {
		t.Fatalf("mode = %v, want 9", result.Result["mode"])
	}
}

func TestInlineModeRejectsNegativeMode(t *testing.T) {
	setupMonitorPipelineTestGraph()
	invokeInlineCommand("!!! add modeProbe")

	result := invokeInlineCommand("!!! mode modeProbe -1")

	if result.Status != InlineCommandStatusRejected {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusRejected, result.Result)
	}
	if result.Result["error"] != "invalid mode" {
		t.Fatalf("error = %v, want invalid mode", result.Result["error"])
	}
}

func TestInlineModeRejectsMissingMatcher(t *testing.T) {
	setupMonitorPipelineTestGraph()

	result := invokeInlineCommand("!!! mode noSuchMatcher 1")

	if result.Status != InlineCommandStatusRejected {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusRejected, result.Result)
	}
	if result.Result["error"] != "matcher not found" {
		t.Fatalf("error = %v, want matcher not found", result.Result["error"])
	}
	if result.Result["matcher_name"] != "noSuchMatcher" {
		t.Fatalf("matcher_name = %v, want noSuchMatcher", result.Result["matcher_name"])
	}
}

func TestInlineModeAcceptsValidMode(t *testing.T) {
	setupMonitorPipelineTestGraph()
	invokeInlineCommand("!!! add modeProbe")

	result := invokeInlineCommand("!!! mode modeProbe 2")

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusApplied, result.Result)
	}
	if result.Result["mode"] != 2 {
		t.Fatalf("mode = %v, want 2", result.Result["mode"])
	}
	matcher := findMatcherByName("modeProbe")
	if matcher == nil {
		t.Fatal("modeProbe matcher was not found")
	}
	if matcher.displayMatchMode != 2 {
		t.Fatalf("displayMatchMode = %d, want 2", matcher.displayMatchMode)
	}
}

func TestInlineModeRejectsExtraArguments(t *testing.T) {
	setupMonitorPipelineTestGraph()
	invokeInlineCommand("!!! add modeProbe")

	result := invokeInlineCommand("!!! mode modeProbe 2 extra")

	if result.Status != InlineCommandStatusRejected {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusRejected, result.Result)
	}
	if result.Result["error"] != "unexpected extra arguments" {
		t.Fatalf("error = %v, want unexpected extra arguments", result.Result["error"])
	}
}

func TestInlineModeAllowsTrailingComment(t *testing.T) {
	setupMonitorPipelineTestGraph()
	invokeInlineCommand("!!! add modeProbe")

	result := invokeInlineCommand("!!! mode modeProbe 2 # useful note")

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusApplied, result.Result)
	}
	matcher := findMatcherByName("modeProbe")
	if matcher == nil {
		t.Fatal("modeProbe matcher was not found")
	}
	if matcher.displayMatchMode != 2 {
		t.Fatalf("displayMatchMode = %d, want 2", matcher.displayMatchMode)
	}
}

func TestInlineColorRejectsMissingMatcher(t *testing.T) {
	setupMonitorPipelineTestGraph()

	result := invokeInlineCommand("!!! color noSuchMatcher red")

	if result.Status != InlineCommandStatusRejected {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusRejected, result.Result)
	}
	if result.Result["error"] != "matcher not found" {
		t.Fatalf("error = %v, want matcher not found", result.Result["error"])
	}
	if result.Result["matcher_name"] != "noSuchMatcher" {
		t.Fatalf("matcher_name = %v, want noSuchMatcher", result.Result["matcher_name"])
	}
}

func TestInlineColorAppliesToExistingMatcher(t *testing.T) {
	setupMonitorPipelineTestGraph()
	invokeInlineCommand("!!! add colorProbe")

	result := invokeInlineCommand("!!! color colorProbe red")

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusApplied, result.Result)
	}
	if result.Result["color"] != "[red]" {
		t.Fatalf("color = %v, want [red]", result.Result["color"])
	}
	matcher := findMatcherByName("colorProbe")
	if matcher == nil {
		t.Fatal("colorProbe matcher was not found")
	}
	if matcher.color != "[red]" {
		t.Fatalf("matcher color = %q, want [red]", matcher.color)
	}
}

func TestInlineColorRejectsInvalidColorIndex(t *testing.T) {
	setupMonitorPipelineTestGraph()
	invokeInlineCommand("!!! add colorProbe")

	result := invokeInlineCommand("!!! color colorProbe 9999")

	if result.Status != InlineCommandStatusRejected {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusRejected, result.Result)
	}
	if result.Result["error"] != "invalid color index" {
		t.Fatalf("error = %v, want invalid color index", result.Result["error"])
	}
	if result.Result["color"] != "9999" {
		t.Fatalf("color = %v, want 9999", result.Result["color"])
	}
}

func TestInlineColorAcceptsValidColorIndex(t *testing.T) {
	setupMonitorPipelineTestGraph()
	invokeInlineCommand("!!! add colorProbe")

	result := invokeInlineCommand("!!! color colorProbe 0")

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusApplied, result.Result)
	}
	if result.Result["color"] != AutobotColors[0] {
		t.Fatalf("color = %v, want %s", result.Result["color"], AutobotColors[0])
	}
	matcher := findMatcherByName("colorProbe")
	if matcher == nil {
		t.Fatal("colorProbe matcher was not found")
	}
	if matcher.color != AutobotColors[0] {
		t.Fatalf("matcher color = %q, want %s", matcher.color, AutobotColors[0])
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
