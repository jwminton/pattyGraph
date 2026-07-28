// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"fmt"
	"strings"
	"testing"
)

func peakLifecycleStats(count int, historyCount int) *WordStats {
	stats := newWordStats()
	stats.count = count
	stats.primeFlux = count
	stats.lastSeenTic = logicalCycles
	stats.historyBuf.Reset()
	stats.historyBuf.Push(historyCount)
	return stats
}

func seedPeakLifecycleCandidate(m *InterestingWordMatcher, key string, count int) {
	m.wordFrequency[key] = peakLifecycleStats(count, count)
}

func seedPeakLifecycleMember(m *InterestingWordMatcher, key string, count int) *WordStats {
	stats := peakLifecycleStats(count, count)
	m.wordFrequency[key] = stats
	m.peakWords = append(m.peakWords, key)
	m.peakWordsSet[key] = true
	m.peakEmptyIntervals[key] = 0
	return stats
}

func TestPeakAdmissionIsBoundedAcrossInterestingFamilies(t *testing.T) {
	for _, family := range []string{"words", "refs", "ips"} {
		t.Run(family, func(t *testing.T) {
			setupMonitorPipelineTestGraph()
			pattyGracePeriod = 2
			logicalCycles = 100
			PattyGraph.linesMatcher.intervalCount = 1000

			var matcher *InterestingWordMatcher
			switch family {
			case "refs":
				matcher = PattyGraph.refsMatcher
			case "ips":
				matcher = PattyGraph.ipsMatcher
			default:
				matcher = PattyGraph.wordsMatcher
			}
			matcher.timeToLive = 1000
			matcher.pushIntervalCount = pattyGracePeriod - 1
			for i := 0; i < peakWordLimit+5; i++ {
				seedPeakLifecycleCandidate(matcher, fmt.Sprintf("key%02d", i), 100)
			}

			matcher.push()

			if len(matcher.peakWords) != peakWordLimit || len(matcher.peakWordsSet) != peakWordLimit {
				t.Fatalf("Peak membership = %d/%d, want %d", len(matcher.peakWords), len(matcher.peakWordsSet), peakWordLimit)
			}
			for i, key := range matcher.peakWords {
				want := fmt.Sprintf("key%02d", i)
				if key != want {
					t.Fatalf("Peak[%d] = %q, want deterministic %q", i, key, want)
				}
			}
			if matcher.peakContentionCount != 5 {
				t.Fatalf("contention = %d, want 5", matcher.peakContentionCount)
			}
		})
	}
}

func TestInterestingMatcherTrackerSupportsMaximumRuntimePeakLimit(t *testing.T) {
	setupMonitorPipelineTestGraph()
	want := InterestingWordListSize + peakWordLimitMax
	if got := PattyGraph.wordsMatcher.topTracker.limit; got != want {
		t.Fatalf("top tracker limit = %d, want %d", got, want)
	}
}

func TestPeakCapacityDoesNotDisplaceExistingMembers(t *testing.T) {
	setupMonitorPipelineTestGraph()
	pattyGracePeriod = 2
	logicalCycles = 100
	PattyGraph.linesMatcher.intervalCount = 1000
	m := PattyGraph.wordsMatcher
	m.timeToLive = 1000
	m.pushIntervalCount = pattyGracePeriod - 1

	for i := 0; i < peakWordLimit; i++ {
		seedPeakLifecycleMember(m, fmt.Sprintf("existing%02d", i), 1)
	}
	seedPeakLifecycleCandidate(m, "strong-contender", 10000)

	m.push()

	if m.peakWordsSet["strong-contender"] {
		t.Fatal("strong contender displaced an existing Peak member")
	}
	if m.peakContentionCount != 1 {
		t.Fatalf("contention = %d, want 1", m.peakContentionCount)
	}
	for i := 0; i < peakWordLimit; i++ {
		key := fmt.Sprintf("existing%02d", i)
		if !m.peakWordsSet[key] {
			t.Fatalf("existing Peak member %q was displaced", key)
		}
	}
}

func TestLowerPeakLimitPreservesExistingMembership(t *testing.T) {
	setupMonitorPipelineTestGraph()
	m := PattyGraph.wordsMatcher
	for i := 0; i < peakWordLimitDefault; i++ {
		seedPeakLifecycleMember(m, fmt.Sprintf("existing%02d", i), 1)
	}

	_, _, _ = setPeakWordLimit(10)
	m.admitPeakCandidates([]peakWordCandidate{{word: "new", strength: 100}})

	if len(m.peakWords) != peakWordLimitDefault {
		t.Fatalf("lowering limit changed membership to %d, want %d", len(m.peakWords), peakWordLimitDefault)
	}
	if m.peakWordsSet["new"] {
		t.Fatal("over-limit family admitted a new Peak")
	}
	if m.peakContentionCount != 1 {
		t.Fatalf("contention = %d, want 1", m.peakContentionCount)
	}
}

func TestRaisingPeakLimitMakesSlotsAvailable(t *testing.T) {
	setupMonitorPipelineTestGraph()
	_, _, _ = setPeakWordLimit(10)
	m := PattyGraph.wordsMatcher
	for i := 0; i < 10; i++ {
		seedPeakLifecycleMember(m, fmt.Sprintf("existing%02d", i), 1)
	}
	m.admitPeakCandidates([]peakWordCandidate{{word: "blocked", strength: 100}})
	if m.peakWordsSet["blocked"] {
		t.Fatal("full family admitted candidate before limit increase")
	}

	_, _, _ = setPeakWordLimit(11)
	m.admitPeakCandidates([]peakWordCandidate{{word: "admitted", strength: 100}})
	if !m.peakWordsSet["admitted"] || len(m.peakWords) != 11 {
		t.Fatalf("raised limit membership = %d/admitted %t, want 11/true", len(m.peakWords), m.peakWordsSet["admitted"])
	}
}

func TestPeakAdmissionPrefersStrongestCandidate(t *testing.T) {
	setupMonitorPipelineTestGraph()
	m := PattyGraph.wordsMatcher
	candidates := make([]peakWordCandidate, 0, peakWordLimit+1)
	for i := 0; i < peakWordLimit; i++ {
		candidates = append(candidates, peakWordCandidate{
			word:     fmt.Sprintf("ordinary%02d", i),
			strength: 1,
		})
	}
	candidates = append(candidates, peakWordCandidate{word: "strongest", strength: 10})

	m.admitPeakCandidates(candidates)

	if !m.peakWordsSet["strongest"] {
		t.Fatal("strongest simultaneous candidate was not admitted")
	}
	if m.peakWordsSet["ordinary24"] {
		t.Fatal("lexically last weak candidate was admitted over strongest candidate")
	}
}

func TestPeakRetiresAfterGraceEmptyIntervals(t *testing.T) {
	setupMonitorPipelineTestGraph()
	doRandomFact = true
	pattyGracePeriod = 3
	logicalCycles = 100
	m := PattyGraph.wordsMatcher
	m.timeToLive = 1000
	stats := seedPeakLifecycleMember(m, "catalog", 0)
	initialChangeState := PattyGraph.changeMatcher.resetState

	m.push()
	if !m.peakWordsSet["catalog"] || stats.historyBuf.Latest() != 0 || m.peakEmptyIntervals["catalog"] != 1 {
		t.Fatalf("first empty interval state = peak %t/latest %d/empty %d", m.peakWordsSet["catalog"], stats.historyBuf.Latest(), m.peakEmptyIntervals["catalog"])
	}

	stats.count = 1
	stats.primeFlux++
	m.push()
	if m.peakEmptyIntervals["catalog"] != 0 || stats.historyBuf.Latest() != 1 {
		t.Fatalf("hit did not reset empty interval state: empty %d/latest %d", m.peakEmptyIntervals["catalog"], stats.historyBuf.Latest())
	}

	for empty := 1; empty < pattyGracePeriod; empty++ {
		m.push()
		if !m.peakWordsSet["catalog"] {
			t.Fatalf("Peak retired after %d empty intervals, want %d", empty, pattyGracePeriod)
		}
	}
	m.push()

	if m.peakWordsSet["catalog"] || len(m.peakWords) != 0 {
		t.Fatal("Peak membership survived the grace-length empty run")
	}
	if _, exists := m.wordFrequency["catalog"]; exists {
		t.Fatal("retired Peak retained stale WordStats and could requalify")
	}
	if m.peakRetiredCount != 1 || m.peakRetirementGrace != pattyGracePeriod {
		t.Fatalf("retirement metadata = %d/%d, want 1/%d", m.peakRetiredCount, m.peakRetirementGrace, pattyGracePeriod)
	}
	if PattyGraph.changeMatcher.resetState != initialChangeState {
		t.Fatal("automatic Peak retirement reset Change")
	}
	lastFact := facts.forced[len(facts.forced)-1]
	if lastFact.Name != "interesting.peakRetirement" {
		t.Fatalf("retirement fact = %q, want interesting.peakRetirement", lastFact.Name)
	}
	if text := stripBrackets(lastFact.Generate(nil)); !strings.Contains(text, "Words Peak retired:1") {
		t.Fatalf("retirement fact text = %q", text)
	}
}

func TestStalePeakContinuesReceivingZeroHistory(t *testing.T) {
	setupMonitorPipelineTestGraph()
	pattyGracePeriod = 3
	logicalCycles = 100
	m := PattyGraph.wordsMatcher
	m.timeToLive = 1
	stats := seedPeakLifecycleMember(m, "quiet", 0)
	stats.lastSeenTic = 0

	m.push()

	if !m.peakWordsSet["quiet"] {
		t.Fatal("stale Peak retired before grace")
	}
	if stats.historyBuf.Latest() != 0 || m.peakEmptyIntervals["quiet"] != 1 {
		t.Fatalf("stale Peak history/empty = %d/%d, want 0/1", stats.historyBuf.Latest(), m.peakEmptyIntervals["quiet"])
	}
}

func TestPeakRetirementUsesCurrentGraceSetting(t *testing.T) {
	setupMonitorPipelineTestGraph()
	logicalCycles = 100
	m := PattyGraph.refsMatcher
	m.timeToLive = 1000
	seedPeakLifecycleMember(m, "old-ref", 0)
	m.peakEmptyIntervals["old-ref"] = 4
	pattyGracePeriod = 5

	m.push()

	if m.peakWordsSet["old-ref"] {
		t.Fatal("Peak retirement ignored the current grace setting")
	}
	if m.peakRetirementGrace != 5 {
		t.Fatalf("retirement grace = %d, want 5", m.peakRetirementGrace)
	}
}

func TestPeakContentionFactoidIsEdgeTriggered(t *testing.T) {
	setupMonitorPipelineTestGraph()
	doRandomFact = true
	m := PattyGraph.wordsMatcher
	initialFacts := len(facts.forced)

	m.updatePeakContention(7)
	if len(facts.forced) != initialFacts+1 {
		t.Fatalf("contention facts = %d, want one new fact", len(facts.forced)-initialFacts)
	}
	first := facts.forced[len(facts.forced)-1]
	if first.Name != "interesting.peakContention" {
		t.Fatalf("contention fact = %q", first.Name)
	}
	if text := stripBrackets(first.Generate(nil)); !strings.Contains(text, "Words Peak full:+7; lower scale") {
		t.Fatalf("contention fact text = %q", text)
	}

	m.updatePeakContention(9)
	if len(facts.forced) != initialFacts+1 {
		t.Fatal("continuous contention emitted a repeated fact")
	}
	m.updatePeakContention(0)
	m.updatePeakContention(2)
	if len(facts.forced) != initialFacts+2 {
		t.Fatal("contention did not emit again after clearing")
	}

	ipMatcher := PattyGraph.ipsMatcher
	ipMatcher.updatePeakContention(4)
	ipFact := facts.forced[len(facts.forced)-1]
	if text := stripBrackets(ipFact.Generate(nil)); !strings.Contains(text, "IPs Peak full:+4; review/purge") {
		t.Fatalf("IP contention fact text = %q", text)
	}
}

func TestPeakFactoidsDoNotAccumulateDuringPreload(t *testing.T) {
	setupMonitorPipelineTestGraph()
	m := PattyGraph.wordsMatcher
	initialFacts := len(facts.forced)

	m.updatePeakContention(7)
	if len(facts.forced) != initialFacts || m.peakContentionActive {
		t.Fatal("preload contention queued or suppressed the later live fact")
	}

	doRandomFact = true
	m.updatePeakContention(7)
	if len(facts.forced) != initialFacts+1 || !m.peakContentionActive {
		t.Fatal("first live contention did not emit after preload")
	}
}

func TestPurgePeakWordsClearsLifecycleState(t *testing.T) {
	setupMonitorPipelineTestGraph()
	m := PattyGraph.wordsMatcher
	seedPeakLifecycleMember(m, "catalog", 0)
	m.peakEmptyIntervals["catalog"] = 2
	m.peakContentionCount = 3
	m.peakContentionActive = true
	m.peakRetiredCount = 1
	m.peakRetirementGrace = 15

	m.purgePeakWords()

	if len(m.peakWords) != 0 || len(m.peakWordsSet) != 0 || len(m.peakEmptyIntervals) != 0 {
		t.Fatal("purge retained Peak membership lifecycle state")
	}
	if m.peakContentionCount != 0 || m.peakContentionActive || m.peakRetiredCount != 0 || m.peakRetirementGrace != 0 {
		t.Fatal("purge retained Peak observation state")
	}
}
