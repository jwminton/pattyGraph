// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestSoftChangeScoreUsesHalfScale(t *testing.T) {
	if got := softChangeScore(0.25, 0.25); math.Abs(got-50) > 1e-12 {
		t.Fatalf("soft score = %v, want 50", got)
	}
	if got := softChangeScore(0, 0.25); got != 0 {
		t.Fatalf("zero soft score = %v, want 0", got)
	}
}

func TestCompareChangeShapesPreservesPattyViewContrast(t *testing.T) {
	reference := changeShape{lines: 1000}
	selected := changeShape{lines: 1250}

	result := compareChangeShapes(reference, selected)
	lineScore := changeComponentScoreForTest(t, result.components, "lines")
	if math.Abs(lineScore-39.024390243902445) > 1e-9 {
		t.Fatalf("line component = %v, want PattyView candidate", lineScore)
	}
	if math.Abs(result.score-15.229030339083882) > 1e-9 {
		t.Fatalf("final score = %v, want PattyView contrast", result.score)
	}
	if result.components[0].key != "lines" {
		t.Fatalf("primary component = %q, want lines", result.components[0].key)
	}
}

func TestCompareChangeShapesUsesCandidateScalarCalibration(t *testing.T) {
	reference := changeShape{
		lines:        1000,
		bytes:        1024 * 1024,
		errors:       10,
		averageBytes: changeOptionalValue{value: 128, available: true},
		marked:       changeOptionalValue{value: 10, available: true},
		b16:          changeOptionalValue{value: 20, available: true},
	}
	selected := changeShape{
		lines:        1250,
		bytes:        1280 * 1024,
		errors:       20,
		averageBytes: changeOptionalValue{value: 160, available: true},
		marked:       changeOptionalValue{value: 14, available: true},
		b16:          changeOptionalValue{value: 32, available: true},
	}

	components := compareChangeShapes(reference, selected).components
	for _, key := range []string{"lines", "bytes", "avg bytes/line"} {
		if got := changeComponentScoreForTest(t, components, key); math.Abs(got-39.024390243902445) > 1e-9 {
			t.Fatalf("%s component = %v, want 39.024390243902445", key, got)
		}
	}
	for _, key := range []string{"errs", "marked", "b16"} {
		if got := changeComponentScoreForTest(t, components, key); math.Abs(got-50) > 1e-12 {
			t.Fatalf("%s component = %v, want 50", key, got)
		}
	}
}

func TestCompareChangeShapesWeightsPeakFamilies(t *testing.T) {
	disjointA := []changeDistributionEntry{{key: "a", count: 100}}
	disjointB := []changeDistributionEntry{{key: "b", count: 100}}

	word := compareChangeShapes(
		changeShape{wordPeaks: disjointA},
		changeShape{wordPeaks: disjointB},
	)
	ref := compareChangeShapes(
		changeShape{refPeaks: disjointA},
		changeShape{refPeaks: disjointB},
	)
	ip := compareChangeShapes(
		changeShape{ipPeaks: disjointA},
		changeShape{ipPeaks: disjointB},
	)

	wordScore := changeComponentScoreForTest(t, word.components, "peak balance")
	refScore := changeComponentScoreForTest(t, ref.components, "peak balance")
	ipScore := changeComponentScoreForTest(t, ip.components, "peak balance")
	if !(wordScore > refScore && refScore > ipScore) {
		t.Fatalf("peak weighting = word %v ref %v ip %v; want word > ref > ip", wordScore, refScore, ipScore)
	}
}

func TestCompareChangeShapesSeparatesWordWaveFromStablePeaks(t *testing.T) {
	reference := changeShape{
		wordPeaks: []changeDistributionEntry{{key: "stable", count: 100}},
		wordWave: []changeDistributionEntry{
			{key: "checkout", count: 90},
			{key: "search", count: 10},
		},
	}
	selected := changeShape{
		wordPeaks: []changeDistributionEntry{{key: "stable", count: 10}},
		wordWave: []changeDistributionEntry{
			{key: "checkout", count: 10},
			{key: "search", count: 90},
		},
	}

	components := compareChangeShapes(reference, selected).components
	if got := changeComponentScoreForTest(t, components, "peak balance"); got != 0 {
		t.Fatalf("stable Peak balance = %v, want 0", got)
	}
	if got := changeComponentScoreForTest(t, components, "word wave"); got <= 60 {
		t.Fatalf("Word wave = %v, want greater than 60", got)
	}
}

func TestChangeDistributionDistanceUsesNormalizedBalance(t *testing.T) {
	proportionalA := []changeDistributionEntry{{key: "catalog", count: 80}, {key: "search", count: 20}}
	proportionalB := []changeDistributionEntry{{key: "catalog", count: 160}, {key: "search", count: 40}}
	shifted := []changeDistributionEntry{{key: "catalog", count: 20}, {key: "search", count: 80}}

	if got := changeDistributionDistance(proportionalA, proportionalB); got != 0 {
		t.Fatalf("proportional distance = %v, want 0", got)
	}
	if got := changeDistributionDistance(proportionalA, shifted); math.Abs(got-0.6) > 1e-12 {
		t.Fatalf("shifted distance = %v, want 0.6", got)
	}
	if got := changeDistributionDistance(nil, shifted); got != 1 {
		t.Fatalf("empty-to-populated distance = %v, want 1", got)
	}
}

func TestBoundedChangeEntriesRanksCandidatesLikeSidecar(t *testing.T) {
	m := NewInterestingWordMatcher("words", 60)
	for i := 0; i < changeTopLimit+5; i++ {
		key := string(rune('a' + i))
		m.wordFrequency[key] = changeWordStatsForTest(i+1, i+10)
		m.lruTracker.MarkSeen(key, i+10)
	}
	m.wordFrequency["alpha"] = changeWordStatsForTest(50, 75)
	m.wordFrequency["aardvark"] = changeWordStatsForTest(50, 75)
	m.lruTracker.MarkSeen("alpha", 75)
	m.lruTracker.MarkSeen("aardvark", 75)

	entries := boundedChangeEntries(m, false)
	if len(entries) != changeTopLimit {
		t.Fatalf("bounded entries = %d, want %d", len(entries), changeTopLimit)
	}
	if entries[0].key != "aardvark" || entries[1].key != "alpha" {
		t.Fatalf("tie ordering = %q, %q; want aardvark, alpha", entries[0].key, entries[1].key)
	}
	for i := 1; i < len(entries); i++ {
		if changeEntryBetter(entries[i], entries[i-1]) {
			t.Fatalf("entries are not sorted best-first at %d", i)
		}
	}
}

func TestChangeComponentRankPreservesPreciseAttributionAcrossRoundedTie(t *testing.T) {
	change := NewChangeMatcher()
	change.scoreReady = true
	change.components = []changeComponent{
		{key: "word wave", score: 39.4, order: changeWordWave},
		{key: "lines", score: 39.1, order: changeLines},
	}
	change.matchCountsMap = map[string]int{"word wave": 39, "lines": 39}

	entries := change.sidecarComponentEntries()
	if len(entries) != 2 || entries[0].Key != "word wave" || entries[0].Rank != 1 {
		t.Fatalf("rounded tie ranking = %#v, want precise primary word wave", entries)
	}
}

func TestChangeMatcherBaselinePreparationAndAlertLifecycle(t *testing.T) {
	setupMonitorPipelineTestGraph()
	change := PattyGraph.changeMatcher
	fluxDepth = 1
	change.AlertAbove.set(0, "")
	setChangeIntervalForTest(1000, 1024*1024, 10, 100, 300)

	prePush()
	prePush()
	if !change.prepared || change.scoreReady {
		t.Fatalf("baseline preparation = prepared %v ready %v; want true, false", change.prepared, change.scoreReady)
	}
	if change.intervalCount != 0 || len(change.matchCountsMap) != 0 {
		t.Fatalf("baseline score/counts = %d/%d, want 0/0", change.intervalCount, len(change.matchCountsMap))
	}
	baselineSnapshot := PattyGraph.SidecarSnapshotBeforePush()
	baselineSidecar := sidecarMatcherByNameForTest(t, baselineSnapshot, ChangeMatcherName)
	if baselineSidecar.IntervalCount != 0 || len(baselineSidecar.TopKeys) != 0 {
		t.Fatalf("baseline sidecar = count %d keys %d, want 0/0", baselineSidecar.IntervalCount, len(baselineSidecar.TopKeys))
	}

	push()
	if len(PattyGraph.pendingAlertTransitions) != 0 {
		t.Fatalf("baseline produced %d alert transitions", len(PattyGraph.pendingAlertTransitions))
	}
	if len(change.history) != 1 || change.history[0] != 0 {
		t.Fatalf("baseline history = %v, want [0]", change.history)
	}

	setChangeIntervalForTest(2000, 3*1024*1024, 35, 600, 200)
	prePush()
	preparedScore := change.intervalCount
	prePush()
	if change.intervalCount != preparedScore || preparedScore <= 0 {
		t.Fatalf("idempotent prepared score = %d, want unchanged positive score", change.intervalCount)
	}
	currentSnapshot := PattyGraph.SidecarSnapshotBeforePush()
	currentSidecar := sidecarMatcherByNameForTest(t, currentSnapshot, ChangeMatcherName)
	if currentSidecar.IntervalCount != preparedScore {
		t.Fatalf("sidecar score = %d, want %d", currentSidecar.IntervalCount, preparedScore)
	}
	if len(currentSidecar.TopKeys) != len(changeComponentNames) {
		t.Fatalf("sidecar components = %d, want %d including zero scores", len(currentSidecar.TopKeys), len(changeComponentNames))
	}
	seenComponents := make(map[string]bool, len(currentSidecar.TopKeys))
	for _, entry := range currentSidecar.TopKeys {
		seenComponents[entry.Key] = true
	}
	for _, name := range changeComponentNames {
		if !seenComponents[name] {
			t.Fatalf("sidecar components omitted %q", name)
		}
	}

	push()
	if len(change.history) != 2 || change.history[1] != preparedScore {
		t.Fatalf("change history = %v, want second score %d", change.history, preparedScore)
	}
	if len(change.matchCountsMap) != len(changeComponentNames) {
		t.Fatalf("retained component count = %d, want %d", len(change.matchCountsMap), len(changeComponentNames))
	}
	if len(PattyGraph.pendingAlertTransitions) != 1 {
		t.Fatalf("alert transitions = %d, want 1", len(PattyGraph.pendingAlertTransitions))
	}
	transition := PattyGraph.pendingAlertTransitions[0]
	if transition.MatcherName != ChangeMatcherName || transition.Value != preparedScore {
		t.Fatalf("change alert = %#v, want matcher %q value %d", transition, ChangeMatcherName, preparedScore)
	}
}

func TestChangeMatcherRebaselinesPeakComponentsAfterPurge(t *testing.T) {
	setupMonitorPipelineTestGraph()
	change := PattyGraph.changeMatcher
	setChangeIntervalForTest(1000, 1024*1024, 10, 100, 300)
	setChangePeakForTest(PattyGraph.wordsMatcher, "steady", 100)

	change.prePush()
	change.push()
	purgePeakWordCommand()
	if change.resetState != changeResetPurged {
		t.Fatalf("reset state = %v, want purged", change.resetState)
	}
	if len(facts.forced) == 0 || facts.forced[len(facts.forced)-1].Name != "model.peakReset" {
		t.Fatalf("purge factoid not queued: %#v", facts.forced)
	}

	change.prePush()
	assertChangeComponentsForTest(t, change.components, false)
	change.push()
	if change.resetState != changeResetRebaseline {
		t.Fatalf("first reset push state = %v, want rebaseline", change.resetState)
	}

	setChangePeakForTest(PattyGraph.wordsMatcher, "rebuilt", 100)
	change.prePush()
	assertChangeComponentsForTest(t, change.components, false)
	change.push()
	if change.resetState != changeResetReady {
		t.Fatalf("second reset push state = %v, want ready", change.resetState)
	}

	setChangePeakForTest(PattyGraph.wordsMatcher, "settled", 100)
	change.prePush()
	assertChangeComponentsForTest(t, change.components, true)
}

func TestSelectedMatcherPurgeDoesNotResetPeakModel(t *testing.T) {
	setupMonitorPipelineTestGraph()
	PattyGraph.selectedMatcher = PattyGraph.linesMatcher
	facts.forced = nil

	if PattyGraph.purgeAllPeakContent() {
		t.Fatal("selected matcher purge reported a Peak reset")
	}
	if PattyGraph.changeMatcher.resetState != changeResetReady || len(facts.forced) != 0 {
		t.Fatalf("selected matcher purge changed reset state/factoids: %v/%d",
			PattyGraph.changeMatcher.resetState, len(facts.forced))
	}
}

func TestChangeMatcherMatchIsNoOp(t *testing.T) {
	setupMonitorPipelineTestGraph()
	change := PattyGraph.changeMatcher
	change.intervalCount = 17
	*currentLine = lineSource{logLine: "should remain untouched"}

	if change.match() {
		t.Fatal("change match returned true")
	}
	if change.intervalCount != 17 || currentLine.logLine != "should remain untouched" {
		t.Fatal("change match mutated interval or line state")
	}
}

func TestChangeMatcherCompletedValueDisplay(t *testing.T) {
	setupMonitorPipelineTestGraph()
	change := PattyGraph.changeMatcher
	change.history = []int{12, 47}
	change.matchCountsMap = map[string]int{"lines": 39, "word wave": 0}
	change.displayMatchMode = 2

	row := change.renderSparklineRow()
	valueDisplay := change.color + "   - " + change.displayColor() + "  47|"
	if !stringsContainAll(row, "change", valueDisplay) {
		t.Fatalf("change row %q does not show an empty current value and latest completed value", row)
	}
	if strings.Contains(row, "12") || strings.Contains(row, upArrow) || strings.Contains(row, downArrow) {
		t.Fatalf("change row %q exposes a prior value or computed direction", row)
	}
	detail := change.renderDetailListing()
	dampedBar := renderBarBraille(dampedChangeComponentScore(39, 47))
	if !stringsContainAll(detail, "lines", "word wave", dampedBar) {
		t.Fatalf("change detail %q does not include retained component bars", detail)
	}
	if strings.Contains(detail, " 39") {
		t.Fatalf("change detail %q exposes an ambiguous numeric component score", detail)
	}
}

func TestDampedChangeComponentScoreSoftensOnlyDisplayIntensity(t *testing.T) {
	if got := dampedChangeComponentScore(80, 25); math.Abs(got-40) > 1e-12 {
		t.Fatalf("damped component = %v, want 40", got)
	}
	if got := dampedChangeComponentScore(80, 100); math.Abs(got-80) > 1e-12 {
		t.Fatalf("full-change component = %v, want 80", got)
	}
	if got := dampedChangeComponentScore(80, 0); got != 0 {
		t.Fatalf("zero-change component = %v, want 0", got)
	}
}

func TestChangeMatcherFirstDetailLevelOmitsGreenComponents(t *testing.T) {
	setupMonitorPipelineTestGraph()
	change := PattyGraph.changeMatcher
	change.history = []int{25}
	change.components = []changeComponent{
		{key: "bytes", score: 60, order: changeBytes},
		{key: "lines", score: 30, order: changeLines},
	}
	change.matchCountsMap = map[string]int{"bytes": 60, "lines": 30}
	change.displayMatchMode = 1

	detail := change.renderDetailListing()
	if !strings.Contains(detail, "bytes") || strings.Contains(detail, "lines") {
		t.Fatalf("first-level Change detail did not filter green component: %q", detail)
	}

	change.displayMatchMode = 2
	change.detailListingCache = ""
	detail = change.renderDetailListing()
	if !stringsContainAll(detail, "bytes", "lines") {
		t.Fatalf("second-level Change detail omitted a component: %q", detail)
	}
}

func TestChangeMatcherDefaultOrderAndProtection(t *testing.T) {
	setupMonitorPipelineTestGraph()
	want := []string{"Bots", "lines", "bytes", "errs", "change", "words", "refs", "ips"}
	if got := matcherNamesForTest(PattyGraph.matchers); !reflect.DeepEqual(got, want) {
		t.Fatalf("matcher order = %v, want %v", got, want)
	}
	result := invokeInlineCommand("!!! del change")
	if result.Status != InlineCommandStatusRejected || matcherIndexByNameForTest(ChangeMatcherName) == -1 {
		t.Fatalf("delete change result = %#v", result)
	}
}

func TestChangeMatcherAllowsGenericAlertThresholdsAboveScoreRange(t *testing.T) {
	setupMonitorPipelineTestGraph()
	result := invokeInlineCommand("!!! alert change above 140")
	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("change alert status = %q, want applied: %#v", result.Status, result.Result)
	}
	if !PattyGraph.changeMatcher.AlertAbove.Enabled || PattyGraph.changeMatcher.AlertAbove.Threshold != 140 {
		t.Fatalf("change alert = %#v, want enabled threshold 140", PattyGraph.changeMatcher.AlertAbove)
	}
}

func setChangeIntervalForTest(lines, bytes, errs, marked, b16 int) {
	PattyGraph.linesMatcher.intervalCount = lines
	PattyGraph.linesMatcher.matchCountsMap["marked"] = marked
	PattyGraph.linesMatcher.matchCountsMap[" b16"] = b16
	PattyGraph.bytesMatcher.intervalCount = bytes
	PattyGraph.errsMatcher.intervalCount = errs
}

func setChangePeakForTest(m *InterestingWordMatcher, key string, count int) {
	m.peakWords = []string{key}
	m.peakWordsSet = map[string]bool{key: true}
	m.wordFrequency[key] = changeWordStatsForTest(count, count)
}

func assertChangeComponentsForTest(t *testing.T, components []changeComponent, wantDistribution bool) {
	t.Helper()
	seen := make(map[string]bool, len(components))
	for _, component := range components {
		seen[component.key] = true
	}
	for _, scalar := range []string{"lines", "bytes", "avg bytes/line", "errs", "marked", "b16"} {
		if !seen[scalar] {
			t.Fatalf("scalar component %q missing from %#v", scalar, components)
		}
	}
	for _, distribution := range []string{"peak balance", "word wave"} {
		if seen[distribution] != wantDistribution {
			t.Fatalf("distribution component %q presence = %v, want %v", distribution, seen[distribution], wantDistribution)
		}
	}
}

func changeWordStatsForTest(count, primeFlux int) *WordStats {
	return &WordStats{
		count:      count,
		primeFlux:  primeFlux,
		historyBuf: &ringBuffer{},
		source:     &lineSource{},
	}
}

func changeComponentScoreForTest(t *testing.T, components []changeComponent, key string) float64 {
	t.Helper()
	for _, component := range components {
		if component.key == key {
			return component.score
		}
	}
	t.Fatalf("component %q not found in %#v", key, components)
	return 0
}

func sidecarMatcherByNameForTest(t *testing.T, snapshot SidecarInterval, name string) SidecarMatcher {
	t.Helper()
	for _, matcher := range snapshot.Matchers {
		if matcher.Name == name {
			return matcher
		}
	}
	t.Fatalf("sidecar matcher %q not found", name)
	return SidecarMatcher{}
}

func stringsContainAll(value string, fragments ...string) bool {
	for _, fragment := range fragments {
		if !strings.Contains(value, fragment) {
			return false
		}
	}
	return true
}
