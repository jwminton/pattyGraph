// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"io"
	"log"
	"reflect"
	"testing"
	"time"
)

func silenceExpectedLogs(t *testing.T) {
	t.Helper()
	previous := log.Writer()
	log.SetOutput(io.Discard)
	t.Cleanup(func() {
		log.SetOutput(previous)
	})
}

// resetRuntimeGlobalsForTest returns package-level runtime state to the same
// baseline a fresh process starts from. Tests may still override individual
// values after setup when exercising a non-default configuration.
func resetRuntimeGlobalsForTest() {
	pattyPushFactor = pattyPushFactorDefault
	pattyGracePeriod = pattyGracePeriodDefault
	pattyScaleFactor = pattyScaleFactorDefault
	fluxDepth = DefaultFluxDepth
	colorIndex = 0
	machineDisplayName = ""
	forceZeroStart = false
	expertMode = false
	generateSidecarJSONL = false
	enableControlFile = false
	sidecarWriteFailures = 0
	doRandomFact = false

	currentCycle = 0
	logicalCycles = 0
	botsMigrated = 0
	uaCardinalityMap = make(map[int]uint64, 20)
	totalAgentTokenCount = 0
	currentLine = &lineSource{}
	lastMonitorMaxBuf = &ringBuffer{}
	lastLinesBuf = &ringBuffer{}
	lastBytesBuf = &ringBuffer{}
	lineCh = nil
	controlFileMonitorStarted = false

	matcherColorMap = make(map[string]string)
	firstColorWins = false
	sparkColorCache = nil
	lastGraceUsed = 0
	displayFreezeCountdown = 0
	displayMod = 1
	timeScaleCache = ""
	PattyGraphBuilderComplex = BuilderComplex{}
	startTime = time.Time{}

	poolNews = 0
	poolGets = 0
	poolReturns = 0
	poolGetsMap = make(map[int]uint64, 20)
	poolGetsPerMatcherMap = make(map[uint64]uint64, 20)

	factoidByName = map[string]*Factoid{}
	factoidHistory = nil
	matcherMarchCount = 0
	tickerBuffer = ""
	tickerVisibleOffset = 0
	bottomPanelCurrent = bottomPanelMatchers
	bottomPanelReturnMode = bottomPanelMatchers
	showMetricsPanelContents = false
	tickerPreamble = defaultTickerBg
	panelBuilder.Reset()

	logLoadDuration = 0
	logLoadLinecount = 0
	logLoadIntervalCount = 0
	logLoadGCCost = 0
	startBytesRead = 0
}

func setupMonitorPipelineTestGraph() {
	resetRuntimeGlobalsForTest()
	PattyGraph = NewMonitor(&MonitorConfig{})
	botsIndex = botsMatcherIndex()
	PattyGraph.logtimeCache = nil
	facts = NewFactoidGenerator()
}

func TestSetupMonitorPipelineTestGraphResetsRuntimeGlobals(t *testing.T) {
	setupMonitorPipelineTestGraph()
	pattyPushFactor = 11
	pattyGracePeriod = 70
	pattyScaleFactor = 4.0
	fluxDepth = 9
	colorIndex = 5
	generateSidecarJSONL = true
	enableControlFile = true
	sidecarWriteFailures = 2
	doRandomFact = true
	matcherColorMap["stale"] = "[red]"
	lastMonitorMaxBuf.Push(99)
	factoidHistory = append(factoidHistory, "stale")
	staleFact := factoidByName["settings.push"]
	facts.forced = append(facts.forced, staleFact)
	bottomPanelCurrent = bottomPanelFactoids
	poolGets = 10

	setupMonitorPipelineTestGraph()

	if pattyPushFactor != pattyPushFactorDefault ||
		pattyGracePeriod != pattyGracePeriodDefault ||
		pattyScaleFactor != pattyScaleFactorDefault ||
		fluxDepth != DefaultFluxDepth ||
		colorIndex != 0 {
		t.Fatal("setup did not restore default settings")
	}
	if generateSidecarJSONL || enableControlFile || sidecarWriteFailures != 0 || doRandomFact {
		t.Fatal("setup did not restore output and control state")
	}
	_, staleColorPresent := matcherColorMap["stale"]
	staleFactPresent := false
	for _, forced := range facts.forced {
		if forced == staleFact {
			staleFactPresent = true
			break
		}
	}
	if staleColorPresent || lastMonitorMaxBuf.Len() != 0 || len(factoidHistory) != 0 || staleFactPresent {
		t.Fatal("setup did not clear retained runtime state")
	}
	if bottomPanelCurrent != bottomPanelMatchers || poolGets != 0 {
		t.Fatal("setup did not restore display and pool state")
	}
}

func standardPipelineLine(ip, path, code, bytesValue, referer, userAgent string) string {
	return ip + ` - - [13/May/2026:14:22:31 -0700] "GET ` + path + ` HTTP/1.1" ` + code + ` ` + bytesValue + ` "` + referer + `" "` + userAgent + `"`
}

func matcherNamesForTest(matchers []MatcherFacade) []string {
	names := make([]string, 0, len(matchers))
	for _, matcher := range matchers {
		names = append(names, matcher.matcherName())
	}
	return names
}

func TestNewMonitorNoConfigStartupMatcherShape(t *testing.T) {
	setupMonitorPipelineTestGraph()

	got := matcherNamesForTest(PattyGraph.matchers)
	want := []string{"Bots", "lines", "bytes", "errs", "words", "refs", "ips"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("matcher order = %v, want %v", got, want)
	}
	if botsIndex != 0 {
		t.Fatalf("botsIndex = %d, want 0", botsIndex)
	}
	if !PattyGraph.botsMatcher.isHistorical {
		t.Fatal("Bots isHistorical = false, want true")
	}
}

func TestMatchValidLineUpdatesMonitorCountersAndSystemObservability(t *testing.T) {
	setupMonitorPipelineTestGraph()
	line := standardPipelineLine(
		"192.0.2.10",
		"/admin",
		"200",
		"1234",
		"https://example.com/start",
		"Mozilla/5.0",
	)

	match(line)

	if PattyGraph.totalLines != 1 || PattyGraph.intervalLines != 1 {
		t.Fatalf("line counters = total %d interval %d; want 1, 1", PattyGraph.totalLines, PattyGraph.intervalLines)
	}
	if PattyGraph.totalBytes != 1234 {
		t.Fatalf("totalBytes = %d, want 1234", PattyGraph.totalBytes)
	}
	if currentLine.request != "GET /admin HTTP/1.1" {
		t.Fatalf("currentLine.request = %q", currentLine.request)
	}
	if currentLine.tokenBandCount == 0 {
		t.Fatal("currentLine.tokenBandCount = 0, want tokenized user-agent")
	}
	if uaCardinalityMap[currentLine.tokenBandCount] != 1 {
		t.Fatalf("uaCardinalityMap[%d] = %d, want 1", currentLine.tokenBandCount, uaCardinalityMap[currentLine.tokenBandCount])
	}
	assertSystemObservabilityForTest(t, 1234, 0, "admin", "example.com", "192.0.2.10")
}

func TestMatchErrorLineUpdatesErrsMatcher(t *testing.T) {
	setupMonitorPipelineTestGraph()
	line := standardPipelineLine(
		"192.0.2.10",
		"/missing",
		"404",
		"512",
		"-",
		"Mozilla/5.0",
	)

	match(line)

	if PattyGraph.errsMatcher.intervalCount != 1 {
		t.Fatalf("errs intervalCount = %d, want 1", PattyGraph.errsMatcher.intervalCount)
	}
	if PattyGraph.errsMatcher.matchCountsMap["404"] != 1 {
		t.Fatalf("errs 404 count = %d, want 1", PattyGraph.errsMatcher.matchCountsMap["404"])
	}
}

func TestBotsWinStillAllowsBelowBotsAndSystemObservability(t *testing.T) {
	setupMonitorPipelineTestGraph()
	rangeWatch := IpsMatcher("range-watch", []string{"192.0."})
	PattyGraph.matchers = insertMatcherBeforeLines(PattyGraph.matchers, rangeWatch)
	botsIndex = botsMatcherIndex()

	line := standardPipelineLine(
		"192.0.2.10",
		"/robots.txt",
		"200",
		"321",
		"https://example.com/bots",
		"Mozilla/5.0 Googlebot/2.1",
	)

	match(line)

	if PattyGraph.botsMatcher.intervalCount != 1 {
		t.Fatalf("Bots intervalCount = %d, want 1", PattyGraph.botsMatcher.intervalCount)
	}
	if rangeWatch.intervalCount != 1 {
		t.Fatalf("below-Bots range matcher intervalCount = %d, want 1", rangeWatch.intervalCount)
	}
	assertSystemObservabilityForTest(t, 321, 0, "robots.txt", "example.com", "192.0.2.10")
}

func TestAboveBotsMatcherWinsBeforeBotsButSystemObservabilityStillRuns(t *testing.T) {
	setupMonitorPipelineTestGraph()
	aboveBots := SimplePredicateMatcher("google-simple", []string{"Googlebot"})
	PattyGraph.matchers = insertMatcherBeforeBots(PattyGraph.matchers, aboveBots)
	botsIndex = botsMatcherIndex()

	line := standardPipelineLine(
		"192.0.2.10",
		"/robots.txt",
		"200",
		"321",
		"https://example.com/bots",
		"Mozilla/5.0 Googlebot/2.1",
	)

	match(line)

	if aboveBots.intervalCount != 1 {
		t.Fatalf("above-Bots matcher intervalCount = %d, want 1", aboveBots.intervalCount)
	}
	if PattyGraph.botsMatcher.intervalCount != 0 {
		t.Fatalf("Bots intervalCount = %d, want 0 when earlier competing matcher wins", PattyGraph.botsMatcher.intervalCount)
	}
	assertSystemObservabilityForTest(t, 321, 0, "robots.txt", "example.com", "192.0.2.10")
}

func assertSystemObservabilityForTest(t *testing.T, bytesValue int, errsCount int, word string, ref string, ip string) {
	t.Helper()
	if PattyGraph.linesMatcher.intervalCount != 1 {
		t.Fatalf("lines intervalCount = %d, want 1", PattyGraph.linesMatcher.intervalCount)
	}
	if PattyGraph.bytesMatcher.intervalCount != bytesValue {
		t.Fatalf("bytes intervalCount = %d, want %d", PattyGraph.bytesMatcher.intervalCount, bytesValue)
	}
	if PattyGraph.errsMatcher.intervalCount != errsCount {
		t.Fatalf("errs intervalCount = %d, want %d", PattyGraph.errsMatcher.intervalCount, errsCount)
	}
	if _, exists := PattyGraph.wordsMatcher.wordFrequency[word]; !exists {
		t.Fatalf("words matcher did not record %q", word)
	}
	if _, exists := PattyGraph.refsMatcher.wordFrequency[ref]; !exists {
		t.Fatalf("refs matcher did not record %q", ref)
	}
	if _, exists := PattyGraph.ipsMatcher.wordFrequency[ip]; !exists {
		t.Fatalf("ips matcher did not record %q", ip)
	}
}
