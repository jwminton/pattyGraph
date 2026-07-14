// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func secondaryTestStats(history []int) *WordStats {
	historyBuf := &ringBuffer{}
	for _, value := range history {
		historyBuf.Push(value)
	}
	return &WordStats{
		count:            8,
		bytes:            4096,
		primeFlux:        18,
		agentDeltaMetric: 0.25,
		historyBuf:       historyBuf,
		lastStatus:       "200",
		source:           &lineSource{},
	}
}

func TestSecondaryEntryMetricViews(t *testing.T) {
	setupMonitorPipelineTestGraph()
	PattyGraph.miniWindowIndex = 0
	stats := secondaryTestStats([]int{2, 4, 6})
	matcher := NewInterestingWordMatcher("words", 60)

	tests := []struct {
		view SecondaryView
		want string
	}{
		{view: SecondaryViewPattyFactor, want: "  2.50"},
		{view: SecondaryViewPrimeFlux, want: "    18"},
		{view: SecondaryViewHistoryDepth, want: "  [3]"},
		{view: SecondaryViewAgentDelta, want: "   25%"},
		{view: SecondaryViewSparkline, want: "▆▅▄   "},
		{view: SecondaryViewBytes, want: "    4K"},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("view_%d", test.view), func(t *testing.T) {
			PattyGraph.secondaryView = test.view
			if got := matcher.secondaryEntryMetric(stats, 2); got != test.want {
				t.Fatalf("secondary metric = %q, want %q", got, test.want)
			}
		})
	}
}

func TestSecondaryEntryMetricIPUsesBurstiness(t *testing.T) {
	setupMonitorPipelineTestGraph()
	PattyGraph.secondaryView = SecondaryViewPattyFactor
	stats := secondaryTestStats([]int{2, 4, 6})
	matcher := WordMatcherFactory("ips")

	if got, want := matcher.secondaryEntryMetric(stats, 2), "  0.02"; got != want {
		t.Fatalf("IP mode 0 metric = %q, want burstiness %q", got, want)
	}
}

func secondaryPrefixFixture() (*InterestingWordMatcher, string) {
	matcher := WordMatcherFactory("ips")
	prefix := "192.0.2."
	for i := 0; i < IpGroupActiveThreshold; i++ {
		ip := fmt.Sprintf("%s%d", prefix, i+1)
		history := []int{1, 2, 3}
		if i == IpGroupActiveThreshold-1 {
			history = []int{4, 5}
		}
		stats := secondaryTestStats(history)
		stats.count = 1
		stats.primeFlux = 5
		stats.bytes = 1024
		stats.agentDeltaMetric = 0.20
		stats.source = &lineSource{
			ip:       ip,
			ipPrefix: prefix,
			logLine:  "first " + ip,
		}
		matcher.wordFrequency[ip] = stats
		matcher.ipScratch.Add(ip, prefix)
	}
	return matcher, prefix
}

func TestSecondaryIPPrefixMetricViews(t *testing.T) {
	tests := []struct {
		view SecondaryView
		want string
	}{
		{view: SecondaryViewPattyFactor, want: "  0.02"},
		{view: SecondaryViewPrimeFlux, want: "    75"},
		{view: SecondaryViewHistoryDepth, want: "[3]"},
		{view: SecondaryViewAgentDelta, want: "   20%"},
		{view: SecondaryViewSparkline, want: "▇▆▄   "},
		{view: SecondaryViewBytes, want: "   15K"},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("view_%d", test.view), func(t *testing.T) {
			setupMonitorPipelineTestGraph()
			PattyGraph.secondaryView = test.view
			PattyGraph.miniWindowIndex = 0
			matcher, prefix := secondaryPrefixFixture()

			display, entries := matcher.displayIpGroups()
			if !reflect.DeepEqual(entries, []string{prefix}) {
				t.Fatalf("displayed prefixes = %v, want [%s]", entries, prefix)
			}
			if !strings.Contains(display, test.want) {
				t.Fatalf("prefix display %q does not contain metric %q", display, test.want)
			}
			if test.view == SecondaryViewHistoryDepth && matcher.ipScratch.prefixDepths[prefix] != 3 {
				t.Fatalf("prefix depth = %d, want 3", matcher.ipScratch.prefixDepths[prefix])
			}
			if test.view == SecondaryViewSparkline {
				got := matcher.ipScratch.prefixHistorAggregateBufs[prefix].Slice()
				want := []int{14, 32, 47}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("tail-aligned prefix history = %v, want %v", got, want)
				}
			}
		})
	}
}

func TestSecondaryTabGlyphOrder(t *testing.T) {
	setupMonitorPipelineTestGraph()
	tests := []struct {
		view  SecondaryView
		glyph string
	}{
		{view: SecondaryViewPattyFactor, glyph: "-"},
		{view: SecondaryViewPrimeFlux, glyph: "/"},
		{view: SecondaryViewHistoryDepth, glyph: "|"},
		{view: SecondaryViewAgentDelta, glyph: "\\"},
		{view: SecondaryViewSparkline, glyph: "="},
		{view: SecondaryViewBytes, glyph: "_"},
	}
	for _, test := range tests {
		if got := test.view.Glyph(); got != test.glyph {
			t.Fatalf("view %d glyph = %q, want %q", test.view, got, test.glyph)
		}
	}
}

func TestSecondaryManualTabProgression(t *testing.T) {
	setupMonitorPipelineTestGraph()
	setUIHook()
	capture := PattyGraph.app.GetInputCapture()
	if capture == nil {
		t.Fatal("TUI input capture was not installed")
	}
	tab := tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)

	PattyGraph.demo = true
	PattyGraph.secondaryView = SecondaryViewHistoryDepth
	capture(tab)
	if PattyGraph.demo {
		t.Fatal("manual Tab did not stop demo mode")
	}
	if PattyGraph.secondaryView != SecondaryViewHistoryDepth {
		t.Fatalf("demo-stop Tab changed view to %d, want %d", PattyGraph.secondaryView, SecondaryViewHistoryDepth)
	}

	for _, want := range []SecondaryView{
		SecondaryViewAgentDelta,
		SecondaryViewSparkline,
		SecondaryViewBytes,
		SecondaryViewPattyFactor,
		SecondaryViewPrimeFlux,
		SecondaryViewHistoryDepth,
	} {
		capture(tab)
		if PattyGraph.secondaryView != want {
			t.Fatalf("manual Tab view = %d, want %d", PattyGraph.secondaryView, want)
		}
	}
}

func TestSecondaryDemoTabProgression(t *testing.T) {
	setupMonitorPipelineTestGraph()
	PattyGraph.demo = true
	PattyGraph.secondaryView = SecondaryViewBytes
	logicalCycles = 9

	incrementCycle()
	if PattyGraph.secondaryView != SecondaryViewPattyFactor {
		t.Fatalf("demo view after cycle 10 = %d, want wrapped view %d", PattyGraph.secondaryView, SecondaryViewPattyFactor)
	}
	for i := 0; i < 10; i++ {
		incrementCycle()
	}
	if PattyGraph.secondaryView != SecondaryViewPrimeFlux {
		t.Fatalf("demo view after cycle 20 = %d, want view %d", PattyGraph.secondaryView, SecondaryViewPrimeFlux)
	}
}

func TestSecondarySourceViewDispatch(t *testing.T) {
	setupMonitorPipelineTestGraph()
	matcher := NewPredicateMatcher("source-test")
	matcher.firstMatchLine = "matcher first source"
	matcher.intervalMatchLine = "matcher interval source"
	matcher.lastMatchLine = "matcher latest source"

	interesting := NewInterestingWordMatcher("words", 60)
	interesting.selectedKey = "selected-key"
	interesting.wordFrequency[interesting.selectedKey] = &WordStats{
		historyBuf:           &ringBuffer{},
		lastStatus:           "200",
		source:               &lineSource{logLine: "interesting first source"},
		firstIntervalLogLine: "interesting interval source",
		lastLogLine:          "interesting latest source",
	}
	PattyGraph.selectedInterestingMatcher = interesting

	tests := []struct {
		view            SecondaryView
		matcherWant     string
		interestingWant string
	}{
		{view: SecondaryViewPattyFactor, matcherWant: "matcher first source", interestingWant: "interesting first source"},
		{view: SecondaryViewPrimeFlux, matcherWant: "matcher interval source", interestingWant: "interesting interval source"},
		{view: SecondaryViewHistoryDepth, matcherWant: "matcher latest source", interestingWant: "interesting latest source"},
		{view: SecondaryViewAgentDelta, matcherWant: "matcher latest source", interestingWant: "interesting latest source"},
		{view: SecondaryViewSparkline, matcherWant: "matcher latest source", interestingWant: "interesting latest source"},
		{view: SecondaryViewBytes, matcherWant: "matcher latest source", interestingWant: "interesting latest source"},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("view_%d", test.view), func(t *testing.T) {
			PattyGraph.secondaryView = test.view
			if got := matcher.displayLogLine(); !strings.Contains(got, test.matcherWant) {
				t.Fatalf("matcher source display = %q, want %q", got, test.matcherWant)
			}
			if got := interesting.displayLogLine(); !strings.Contains(got, test.interestingWant) {
				t.Fatalf("interesting source display = %q, want %q", got, test.interestingWant)
			}
		})
	}
}

func TestWordStatsFirstObservationAccountsForBytes(t *testing.T) {
	setupMonitorPipelineTestGraph()
	*currentLine = lineSource{
		logLine:    "first observation",
		bytesValue: 1234,
		respCode:   "200",
	}

	t.Run("new", func(t *testing.T) {
		stats := newWordStats()
		if stats.bytes != uint64(currentLine.bytesValue) {
			t.Fatalf("first-observation bytes = %d, want %d", stats.bytes, currentLine.bytesValue)
		}
	})

	t.Run("repopulated", func(t *testing.T) {
		stats := &WordStats{historyBuf: &ringBuffer{}, source: &lineSource{}}
		repopulateWordStats(stats)
		if stats.bytes != uint64(currentLine.bytesValue) {
			t.Fatalf("repopulated first-observation bytes = %d, want %d", stats.bytes, currentLine.bytesValue)
		}
	})
}

func TestMatcherSourceExamplesFollowLifecycle(t *testing.T) {
	setupMonitorPipelineTestGraph()
	matcher := NewPredicateMatcher("source-lifecycle")
	matcher.predicateFuncs = append(matcher.predicateFuncs, func() (bool, [][]string) {
		return true, nil
	})

	*currentLine = lineSource{logLine: "matcher first", ip: "192.0.2.1", ipPrefix: "192.0.2.", respCode: "200"}
	matcher.match()
	if matcher.firstMatchLine != "matcher first" || matcher.intervalMatchLine != "matcher first" || matcher.lastMatchLine != "matcher first" {
		t.Errorf("one-hit matcher sources = first %q interval %q latest %q; want first observation in all three",
			matcher.firstMatchLine, matcher.intervalMatchLine, matcher.lastMatchLine)
	}

	*currentLine = lineSource{logLine: "matcher second", ip: "192.0.2.2", ipPrefix: "192.0.2.", respCode: "200"}
	matcher.match()
	if matcher.firstMatchLine != "matcher first" || matcher.intervalMatchLine != "matcher first" || matcher.lastMatchLine != "matcher second" {
		t.Errorf("same-interval matcher sources = first %q interval %q latest %q",
			matcher.firstMatchLine, matcher.intervalMatchLine, matcher.lastMatchLine)
	}

	matcher.push()
	if matcher.firstMatchLine != "matcher first" || matcher.intervalMatchLine != "" || matcher.lastMatchLine != "matcher second" {
		t.Errorf("post-push matcher sources = first %q interval %q latest %q; want retained first/latest and empty current interval",
			matcher.firstMatchLine, matcher.intervalMatchLine, matcher.lastMatchLine)
	}
	*currentLine = lineSource{logLine: "matcher third", ip: "192.0.2.3", ipPrefix: "192.0.2.", respCode: "200"}
	matcher.match()
	if matcher.firstMatchLine != "matcher first" || matcher.intervalMatchLine != "matcher third" || matcher.lastMatchLine != "matcher third" {
		t.Errorf("next-interval matcher sources = first %q interval %q latest %q; want first, current interval first, latest",
			matcher.firstMatchLine, matcher.intervalMatchLine, matcher.lastMatchLine)
	}
}

func TestInterestingSourceExamplesFollowLifecycle(t *testing.T) {
	setupMonitorPipelineTestGraph()
	const key = "selected-key"
	matcher := NewInterestingWordMatcher("words", 60)
	matcher.lineParser = func() string { return key }
	matcher.lineTokenizer = func(string) []string { return []string{key} }

	*currentLine = lineSource{logLine: "interesting first", respCode: "200"}
	matcher.match()
	stats := matcher.wordFrequency[key]
	if stats.source.logLine != "interesting first" || stats.firstIntervalLogLine != "interesting first" || stats.lastLogLine != "interesting first" {
		t.Errorf("one-hit interesting sources = first %q interval %q latest %q; want first observation in all three",
			stats.source.logLine, stats.firstIntervalLogLine, stats.lastLogLine)
	}

	*currentLine = lineSource{logLine: "interesting second", respCode: "200"}
	matcher.match()
	if stats.source.logLine != "interesting first" || stats.firstIntervalLogLine != "interesting first" || stats.lastLogLine != "interesting second" {
		t.Errorf("same-interval interesting sources = first %q interval %q latest %q",
			stats.source.logLine, stats.firstIntervalLogLine, stats.lastLogLine)
	}

	matcher.push()
	if stats.source.logLine != "interesting first" || stats.firstIntervalLogLine != "" || stats.lastLogLine != "interesting second" {
		t.Errorf("post-push interesting sources = first %q interval %q latest %q; want retained first/latest and empty current interval",
			stats.source.logLine, stats.firstIntervalLogLine, stats.lastLogLine)
	}
	*currentLine = lineSource{logLine: "interesting third", respCode: "200"}
	matcher.match()
	if stats.source.logLine != "interesting first" || stats.firstIntervalLogLine != "interesting third" || stats.lastLogLine != "interesting third" {
		t.Errorf("next-interval interesting sources = first %q interval %q latest %q; want first, current interval first, latest",
			stats.source.logLine, stats.firstIntervalLogLine, stats.lastLogLine)
	}
}
