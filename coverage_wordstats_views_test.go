// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"fmt"
	"strings"
	"testing"
)

func coverageWordStats(counts ...int) *WordStats {
	ws := blankWordStats()
	ws.Reset()
	ws.count = 3
	ws.primeFlux = 3
	ws.lastStatus = "200"
	ws.source.logLine = "GET /catalog?page=4 HTTP/1.1"
	ws.source.captureColor = "[orange]"
	ws.source.captureMatcher = "coverage"
	ws.firstIntervalLogLine = "first interval catalog"
	ws.lastLogLine = "last catalog"
	for _, count := range counts {
		ws.historyBuf.Push(count)
	}
	return ws
}

func TestCoverageWordStatsLifecycleAndMetrics(t *testing.T) {
	setupMonitorPipelineTestGraph()
	ws := coverageWordStats(0, 2, 8, 4)
	defer recycleWordStats(ws)
	PattyGraph.intervalsCompleted = ws.historyLength()

	if ws.allHistoryIndicator() != "+" || ws.captureColor() != "orange" || ws.pattyMonoColor() == "" {
		t.Fatal("WordStats display metadata mismatch")
	}
	if ws.historyAt(1) != 2 || len(ws.historySlice()) != 4 || len(ws.reversedHistorySlice()) != 4 {
		t.Fatal("WordStats history access mismatch")
	}
	if ws.historyTotal() != 14 || ws.normalized() <= 0 || ws.burstiness() <= 0 {
		t.Fatal("WordStats derived metrics were not calculated")
	}
	ws.retainAgentTokenBaseline([]string{"Mozilla", "coverage"})
	if len(ws.agentTokensFromSource) != 2 {
		t.Fatal("agent token baseline was not retained")
	}
	ws.push()
	if ws.count != 0 || ws.bytes != 0 || ws.firstIntervalLogLine != "" {
		t.Fatal("WordStats push did not reset interval state")
	}

	empty := blankWordStats()
	if empty.historyTotal() != 0 || empty.normalized() != 0 || empty.allHistoryIndicator() != " " {
		t.Fatal("blank WordStats defaults changed")
	}
	recycleWordStats(empty)
	recycleWordStats(nil)
}

func TestCoverageInterestingMatcherRenderingAndSelection(t *testing.T) {
	setupMonitorPipelineTestGraph()
	m := PattyGraph.wordsMatcher
	PattyGraph.linesMatcher.intervalCount = 100
	PattyGraph.intervalsCompleted = 4
	lastMonitorMaxBuf.Push(20)

	peak := coverageWordStats(3, 8, 4)
	plain := coverageWordStats(1, 2, 1)
	plain.forcedColor = "[blue]"
	m.wordFrequency["catalog"] = peak
	m.wordFrequency["page=4"] = plain
	m.lruTracker.MarkSeen("catalog", peak.primeFlux)
	m.lruTracker.MarkSeen("page=4", plain.primeFlux)
	m.peakWords = []string{"catalog"}
	m.peakWordsSet["catalog"] = true
	m.pushIntervalCount = 4
	defer recycleWordStats(peak)
	defer recycleWordStats(plain)

	if output := m.renderDetailListing(); !strings.Contains(output, "catalog") || !strings.Contains(output, "page=4") {
		t.Fatalf("interesting listing omitted entries: %q", output)
	}
	PattyGraph.selectedInterestingMatcher = m
	m.selectedKey = "catalog"
	if !m.selected() || m.selectionKey() != "catalog" || m.selectedStats() != peak {
		t.Fatal("interesting selection was not retained")
	}
	if row := m.renderSparklineRow(); !strings.Contains(row, "catalog") {
		t.Fatalf("selected sparkline row omitted key: %q", row)
	}

	for _, view := range []SecondaryView{SecondaryViewPattyFactor, SecondaryViewPrimeFlux, SecondaryViewHistoryDepth, SecondaryViewAgentDelta, SecondaryViewSparkline, SecondaryViewBytes} {
		PattyGraph.secondaryView = view
		if line := m.displayLogLine(); line == "" {
			t.Fatalf("view %v omitted selected source line", view)
		}
		_ = m.secondaryEntryMetric(peak, 100)
	}
	if got := m.selectedHistoryAt(0); got == 0 {
		t.Fatal("selected history lookup returned zero")
	}
	m.selectedKey = "missing"
	if !strings.Contains(m.renderSparklineRow(), "missing") || m.displayLogLine() != "" {
		t.Fatal("missing selected key fallback changed")
	}
}

func TestCoverageInterestingMatcherTrackers(t *testing.T) {
	top := NewTopWordTracker(3)
	top.MaybeForSureAdd(wordEntry{word: "three", count: 3})
	top.MaybeForSureAdd(wordEntry{word: "one", count: 1})
	top.MaybeForSureAdd(wordEntry{word: "two", count: 2})
	top.MaybeForSureAdd(wordEntry{word: "four", count: 4})
	if len(top.Top()) != 3 {
		t.Fatalf("top-word tracker length = %d", len(top.Top()))
	}
	top.Reset()

	prefixes := NewTopPrefixTracker(2)
	for _, entry := range []prefixCount{{"a", 1, 1}, {"b", 3, 3}, {"c", 2, 2}} {
		if prefixes.ShouldConsider(entry.countPlusFirst) {
			prefixes.MaybeAdd(entry)
		}
	}
	if len(prefixes.Top()) != 2 {
		t.Fatalf("prefix tracker length = %d", len(prefixes.Top()))
	}
	prefixes.Reset()

	lru := NewScoredLRUTracker(3)
	lru.MarkSeen("a", 1)
	lru.MarkSeen("b", 2)
	lru.MarkSeen("c", 3)
	lru.MarkSeen("a", 4)
	lru.MarkSeen("weak", 0)
	lru.MarkSeen("strong", 5)
	if len(lru.TopN(2)) != 2 {
		t.Fatal("LRU top-N failed")
	}
	lru.Delete("missing")
	lru.Delete("b")
	lru.EvictBelow(4)
	lru.EvictBottomPercent(0)
	lru.EvictBottomPercent(2)
	lru.MarkSeen("x", 6)
	lru.MarkSeen("y", 7)
	lru.EvictBottomPercent(0.5)
}

func TestCoverageIPGroupLifecycleAndHistory(t *testing.T) {
	setupMonitorPipelineTestGraph()
	s := createIpScratch()
	prefix := "192.0.2"
	for i := 1; i <= IpGroupActiveThreshold; i++ {
		s.Add(fmt.Sprintf("192.0.2.%d", i), prefix)
	}
	s.Add("192.0.2.1", prefix)
	if len(s.ActivePrefixes()) != 1 || len(s.ActivePrefixMembers(prefix)) != IpGroupActiveThreshold {
		t.Fatal("IP prefix did not become active")
	}
	s.Remove("192.0.2.1", prefix)
	if len(s.ActivePrefixes()) != 0 {
		t.Fatal("IP prefix remained active below threshold")
	}
	for i := 2; i <= IpGroupActiveThreshold; i++ {
		s.Remove(fmt.Sprintf("192.0.2.%d", i), prefix)
	}
	if members := s.ActivePrefixMembers("missing"); members != nil {
		t.Fatalf("missing prefix returned members: %#v", members)
	}
	s.Add_old("198.51.100.1", "198.51.100")
	s.Remove_old("198.51.100.1", "198.51.100")

	m := PattyGraph.ipsMatcher
	m.selectedKey = "203.0.113"
	aggregate := &ringSeriesAccumulator{len: 3}
	aggregate.data[0], aggregate.data[1], aggregate.data[2] = 2, 4, 6
	m.ipScratch.prefixHistorAggregateBufs[m.selectedKey] = aggregate
	if got := m.selectedHistoryAt(0); got != 6 {
		t.Fatalf("group history = %d, want 6", got)
	}
	if got := m.selectedHistoryAt(99); got != 0 {
		t.Fatalf("out-of-range history = %d", got)
	}
}

func TestCoverageInterestingMaintenance(t *testing.T) {
	setupMonitorPipelineTestGraph()
	m := PattyGraph.wordsMatcher
	for i := 0; i < 20; i++ {
		key := fmt.Sprintf("key-%02d", i)
		m.wordFrequency[key] = coverageWordStats(i)
		m.lruTracker.MarkSeen(key, i)
	}
	m.compactFrequencyMap()
	if len(m.wordFrequency) != 20 {
		t.Fatal("frequency compaction lost entries")
	}

	PattyGraph.intervalsCompleted = pattyGracePeriod - 1
	m.updatePeakWordStats()
	PattyGraph.intervalsCompleted = pattyGracePeriod
	m.peakWords = []string{"one", "two"}
	for i := 0; i < DefaultHistoryDepth+2; i++ {
		m.updatePeakWordStats()
	}
	if len(m.peakWordCounts) != DefaultHistoryDepth || m.averagePeakWords() != 2 {
		t.Fatalf("peak history mismatch: %d %.1f", len(m.peakWordCounts), m.averagePeakWords())
	}

	for _, ws := range m.wordFrequency {
		recycleWordStats(ws)
	}
}
