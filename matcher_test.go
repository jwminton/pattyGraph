// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"strings"
	"testing"
)

func setupMatcherTestGraph() *Matcher {
	bots := NewPredicateMatcher("Bots")
	PattyGraph = &Monitor{
		botsMatcher: bots,
	}
	*currentLine = lineSource{}
	return bots
}

func setMatcherTestLine(logLine string) {
	*currentLine = lineSource{
		logLine:        logLine,
		ip:             stringInterner.Intern("192.0.2.10"),
		ipPrefix:       prefixInterner.Intern("192.0."),
		request:        "GET /admin HTTP/1.1",
		referer:        "https://example.com/start",
		userAgent:      "Mozilla/5.0 SuspiciousBot/1.0",
		respCode:       "200",
		bytesValue:     1234,
		tokenBandCount: 6,
	}
}

func TestSimplePredicateMatcherMatchTracksCountsAndLines(t *testing.T) {
	setupMatcherTestGraph()
	setMatcherTestLine(`192.0.2.10 - - [13/May/2026:14:22:31 -0700] "GET /admin HTTP/1.1" 200 1234 "-" "SuspiciousBot"`)
	m := SimplePredicateMatcher("SuspiciousBot", []string{"SuspiciousBot"})
	m.setColor("[red]")

	if !m.match() {
		t.Fatal("match() = false, want true")
	}
	if m.intervalCount != 1 {
		t.Fatalf("intervalCount = %d, want 1", m.intervalCount)
	}
	if m.bytesServed != 1234 {
		t.Fatalf("bytesServed = %d, want 1234", m.bytesServed)
	}
	if m.matchCountsMap[currentLine.ip] != 1 {
		t.Fatalf("matchCountsMap[ip] = %d, want 1", m.matchCountsMap[currentLine.ip])
	}
	if m.ipGroupsMap[currentLine.ipPrefix] != 1 {
		t.Fatalf("ipGroupsMap[prefix] = %d, want 1", m.ipGroupsMap[currentLine.ipPrefix])
	}
	if m.ipGroupsCountsMap[currentLine.ipPrefix] != 1 {
		t.Fatalf("ipGroupsCountsMap[prefix] = %d, want 1", m.ipGroupsCountsMap[currentLine.ipPrefix])
	}
	if m.firstMatchLine != currentLine.logLine || m.intervalMatchLine != currentLine.logLine || m.lastMatchLine != currentLine.logLine {
		t.Fatalf("match lines were not captured from currentLine")
	}
	if currentLine.captureColor != "[red]" {
		t.Fatalf("captureColor = %q, want [red]", currentLine.captureColor)
	}
}

func TestMatcherStickyIPCountsRelatedLineWithoutOfficialMatch(t *testing.T) {
	setupMatcherTestGraph()
	setMatcherTestLine(`192.0.2.10 - - [13/May/2026:14:22:31 -0700] "GET /admin HTTP/1.1" 200 1234 "-" "SuspiciousBot"`)
	m := SimplePredicateMatcher("SuspiciousBot", []string{"SuspiciousBot"})

	if !m.match() {
		t.Fatal("first match() = false, want true")
	}
	firstMatchLine := m.lastMatchLine

	setMatcherTestLine(`192.0.2.10 - - [13/May/2026:14:22:32 -0700] "GET /other HTTP/1.1" 200 100 "-" "Mozilla"`)
	currentLine.bytesValue = 100
	if m.match() {
		t.Fatal("second match() = true, want false for sticky IP accounting")
	}
	if m.intervalCount != 2 {
		t.Fatalf("intervalCount = %d, want 2", m.intervalCount)
	}
	if m.matchCountsMap[currentLine.ip] != 2 {
		t.Fatalf("matchCountsMap[ip] = %d, want 2", m.matchCountsMap[currentLine.ip])
	}
	if m.ipGroupsMap[currentLine.ipPrefix] != 1 {
		t.Fatalf("ipGroupsMap[prefix] = %d, want 1", m.ipGroupsMap[currentLine.ipPrefix])
	}
	if m.lastMatchLine != firstMatchLine {
		t.Fatalf("lastMatchLine changed on sticky non-match")
	}
}

func TestIpsMatcherMatchesFullIpAndPrefix(t *testing.T) {
	setupMatcherTestGraph()
	setMatcherTestLine(`192.0.2.10 - - [13/May/2026:14:22:31 -0700] "GET / HTTP/1.1" 200 12 "-" "curl"`)

	exact := IpsMatcher("exact-ip", []string{"192.0.2.10"})
	if !exact.match() {
		t.Fatal("exact IP matcher did not match currentLine.ip")
	}

	prefix := IpsMatcher("prefix-ip", []string{"192.0."})
	if !prefix.match() {
		t.Fatal("prefix IP matcher did not match currentLine.ipPrefix")
	}
}

func TestMatcherFactorySystemMatchersUseSpecialCounters(t *testing.T) {
	setupMatcherTestGraph()
	setMatcherTestLine(`192.0.2.10 - - [13/May/2026:14:22:31 -0700] "GET /admin HTTP/1.1" 404 1234 "-" "SuspiciousBot"`)
	currentLine.respCode = "404"
	currentLine.captureColor = "[blue]"

	lines := MatcherFactory("lines")
	if !lines.match() {
		t.Fatal("lines matcher did not match")
	}
	if lines.intervalCount != 1 || lines.matchCountsMap["----"] != 1 {
		t.Fatalf("lines counts = interval %d, ---- %d; want 1, 1", lines.intervalCount, lines.matchCountsMap["----"])
	}
	if lines.matchCountsMap["marked"] != 1 {
		t.Fatalf("lines marked count = %d, want 1", lines.matchCountsMap["marked"])
	}
	if lines.matchCountsMap["1–10K"] != 1 {
		t.Fatalf("lines size bucket count = %d, want 1", lines.matchCountsMap["1–10K"])
	}

	bytes := MatcherFactory("bytes")
	if !bytes.match() {
		t.Fatal("bytes matcher did not match")
	}
	if bytes.intervalCount != 1234 || bytes.matchCountsMap["----"] != 1234 {
		t.Fatalf("bytes counts = interval %d, ---- %d; want 1234, 1234", bytes.intervalCount, bytes.matchCountsMap["----"])
	}

	errs := MatcherFactory("errs")
	if !errs.match() {
		t.Fatal("errs matcher did not match 404")
	}
	if errs.intervalCount != 1 || errs.matchCountsMap["404"] != 1 {
		t.Fatalf("errs counts = interval %d, 404 %d; want 1, 1", errs.intervalCount, errs.matchCountsMap["404"])
	}
}

func TestMatcherPushResetsIntervalAndTrimsHistory(t *testing.T) {
	setupMatcherTestGraph()
	m := NewPredicateMatcher("test")
	for i := 0; i < DefaultHistoryDepth; i++ {
		m.history = append(m.history, i)
	}
	m.intervalCount = 7
	m.matchCountsMap["192.0.2.10"] = 3
	m.ipGroupsCountsMap["192.0."] = 3

	m.push()

	if len(m.history) != DefaultHistoryDepth {
		t.Fatalf("history len = %d, want %d", len(m.history), DefaultHistoryDepth)
	}
	if m.history[0] != 1 {
		t.Fatalf("history[0] = %d, want 1 after trimming oldest value", m.history[0])
	}
	if got := m.history[len(m.history)-1]; got != 7 {
		t.Fatalf("last history value = %d, want pushed interval 7", got)
	}
	if m.lastIntervalCount != 7 || m.intervalCount != 0 {
		t.Fatalf("counts after push = last %d interval %d; want 7, 0", m.lastIntervalCount, m.intervalCount)
	}
	if m.matchCountsMap["192.0.2.10"] != 0 {
		t.Fatalf("matchCountsMap was not reset")
	}
	if m.ipGroupsCountsMap["192.0."] != 0 {
		t.Fatalf("ipGroupsCountsMap was not reset")
	}
}

func TestPlaceMatcherPreservesLayeringAndRefreshesOrderState(t *testing.T) {
	bots := NewPredicateMatcher("Bots")
	lines := NewPredicateMatcher("lines")
	PattyGraph = &Monitor{
		botsMatcher:  bots,
		linesMatcher: lines,
		matchers:     []MatcherFacade{bots, lines},
		overallMax:   42,
	}
	matcherColorMap = make(map[string]string)
	bots.historySparklineCache = "stale"
	lines.historySparklineCache = "stale"

	top := NewPredicateMatcher("top")
	if !placeMatcher(top, matcherFirst) {
		t.Fatal("placeMatcher rejected first placement")
	}
	if PattyGraph.matchers[0] != top {
		t.Fatalf("placeMatcher did not place matcher first")
	}
	if !top.isHistorical {
		t.Fatalf("top matcher isHistorical = false, want true")
	}
	if botsIndex != 1 {
		t.Fatalf("botsIndex = %d, want 1 after first placement", botsIndex)
	}
	if PattyGraph.overallMax != -1 {
		t.Fatalf("overallMax = %d, want cache invalidation sentinel -1", PattyGraph.overallMax)
	}
	if bots.historySparklineCache != "" || lines.historySparklineCache != "" {
		t.Fatal("placement did not clear matcher sparkline caches")
	}
	if matcherColorMap[top.name] == "" || top.color != matcherColorMap[top.name] {
		t.Fatalf("top color = %q, remembered color = %q", top.color, matcherColorMap[top.name])
	}

	beforeBots := NewPredicateMatcher("before-bots")
	if !placeMatcher(beforeBots, matcherBeforeBots) {
		t.Fatal("placeMatcher rejected before-Bots placement")
	}
	if PattyGraph.matchers[1] != beforeBots || PattyGraph.matchers[2] != bots {
		t.Fatalf("before-Bots order = %s, %s; want before-bots, Bots", PattyGraph.matchers[1].matcherName(), PattyGraph.matchers[2].matcherName())
	}
	if !beforeBots.isHistorical {
		t.Fatalf("before-bots matcher isHistorical = false, want true")
	}
	if botsIndex != 2 {
		t.Fatalf("botsIndex = %d, want 2 after before-Bots placement", botsIndex)
	}

	beforeLines := NewPredicateMatcher("before-lines")
	if !placeMatcher(beforeLines, matcherBeforeLines) {
		t.Fatal("placeMatcher rejected before-lines placement")
	}
	if PattyGraph.matchers[3] != beforeLines || PattyGraph.matchers[4] != lines {
		t.Fatalf("before-lines order = %s, %s; want before-lines, lines", PattyGraph.matchers[3].matcherName(), PattyGraph.matchers[4].matcherName())
	}
	if beforeLines.isHistorical {
		t.Fatalf("before-lines matcher isHistorical = true, want false")
	}
}

func TestPlaceMatcherRejectsMissingPlacementAnchorWithoutMutation(t *testing.T) {
	bots := NewPredicateMatcher(BotsMatcherName)
	PattyGraph = &Monitor{
		botsMatcher: bots,
		matchers:    []MatcherFacade{bots},
	}

	original := append([]MatcherFacade(nil), PattyGraph.matchers...)
	if placeMatcher(NewPredicateMatcher("observer"), matcherBeforeLines) {
		t.Fatal("placeMatcher accepted placement without the lines anchor")
	}
	if len(PattyGraph.matchers) != len(original) || PattyGraph.matchers[0] != original[0] {
		t.Fatal("failed placement mutated the matcher slice")
	}
}

func TestBotsCapturesConcreteBotToken(t *testing.T) {
	setupMonitorPipelineTestGraph()
	line := standardPipelineLine(
		"192.0.2.10",
		"/robots.txt",
		"200",
		"321",
		"-",
		"Mozilla/5.0 Googlebot",
	)

	match(line)

	if PattyGraph.botsMatcher.intervalCount != 1 {
		t.Fatalf("Bots intervalCount = %d, want 1", PattyGraph.botsMatcher.intervalCount)
	}
	if PattyGraph.botsMatcher.matchCountsMap["Googlebot"] != 1 {
		t.Fatalf("Bots Googlebot count = %d, want 1", PattyGraph.botsMatcher.matchCountsMap["Googlebot"])
	}
}

func TestBotsMigrationPromotesTopBotBeforeBots(t *testing.T) {
	setupMonitorPipelineTestGraph()
	line := standardPipelineLine(
		"192.0.2.10",
		"/robots.txt",
		"200",
		"321",
		"-",
		"Mozilla/5.0 Googlebot",
	)
	match(line)

	PattyGraph.botsMatcher.migrateBots(-1)
	botsIndex = botsMatcherIndex()

	if botsMigrated != 1 {
		t.Fatalf("botsMigrated = %d, want 1", botsMigrated)
	}
	if botsIndex != 1 {
		t.Fatalf("botsIndex = %d, want 1 after migrated matcher insertion", botsIndex)
	}
	migrated := PattyGraph.matchers[0].asMatcher()
	if migrated == nil || migrated.matcherName() != "Googlebot" {
		t.Fatalf("first matcher = %v, want Googlebot matcher", PattyGraph.matchers[0].matcherName())
	}
	if migrated.intervalCount != 1 {
		t.Fatalf("migrated intervalCount = %d, want transferred count 1", migrated.intervalCount)
	}
	if PattyGraph.botsMatcher.intervalCount != 0 {
		t.Fatalf("Bots intervalCount = %d, want 0 after migration transfer", PattyGraph.botsMatcher.intervalCount)
	}
	if _, exists := PattyGraph.botsMatcher.matchCountsMap["Googlebot"]; exists {
		t.Fatal("Bots still has Googlebot in matchCountsMap after migration")
	}

	match(line)

	if migrated.intervalCount != 2 {
		t.Fatalf("migrated intervalCount after second match = %d, want 2", migrated.intervalCount)
	}
	if PattyGraph.botsMatcher.intervalCount != 0 {
		t.Fatalf("Bots intervalCount after migrated matcher wins = %d, want 0", PattyGraph.botsMatcher.intervalCount)
	}
}

func TestBotsMigrationRefreshesBoundaryBeforeNextMatch(t *testing.T) {
	setupMonitorPipelineTestGraph()
	line := standardPipelineLine(
		"192.0.2.10",
		"/robots.txt",
		"200",
		"321",
		"-",
		"Mozilla/5.0 Googlebot",
	)
	match(line)

	PattyGraph.botsMatcher.migrateBots(-1)

	if botsIndex != botsMatcherIndex() {
		t.Fatalf("botsIndex = %d, want refreshed index %d after migration", botsIndex, botsMatcherIndex())
	}

	match(line)

	if got := PattyGraph.matchers[0].getCount(); got != 2 {
		t.Fatalf("promoted matcher interval count = %d, want 2", got)
	}
	if got := PattyGraph.botsMatcher.intervalCount; got != 0 {
		t.Fatalf("Bots intervalCount after promoted matcher wins = %d, want 0", got)
	}
	if _, exists := PattyGraph.botsMatcher.matchCountsMap["Googlebot"]; exists {
		t.Fatal("Bots re-collected Googlebot after promoted matcher won")
	}
}

func TestBotsMigrationInvalidatesMatchedDisplayCache(t *testing.T) {
	setupMonitorPipelineTestGraph()
	line := standardPipelineLine(
		"192.0.2.10",
		"/robots.txt",
		"200",
		"321",
		"-",
		"Mozilla/5.0 Googlebot",
	)
	match(line)

	before := PattyGraph.botsMatcher.displayMatched()
	if !strings.Contains(before, "Googlebot") {
		t.Fatalf("pre-migration Bots display = %q, want Googlebot", before)
	}

	PattyGraph.botsMatcher.migrateBots(-1)

	after := PattyGraph.botsMatcher.displayMatched()
	if strings.Contains(after, "Googlebot") {
		t.Fatalf("post-migration Bots display still contains Googlebot: %q", after)
	}
}

func TestPushAutoMigratesBotsWinnerDuringStartup(t *testing.T) {
	setupMonitorPipelineTestGraph()
	line := standardPipelineLine(
		"192.0.2.10",
		"/robots.txt",
		"200",
		"321",
		"-",
		"Mozilla/5.0 Googlebot",
	)
	match(line)

	push()
	botsIndex = botsMatcherIndex()

	if botsMigrated != 1 {
		t.Fatalf("botsMigrated = %d, want 1", botsMigrated)
	}
	if botsIndex != 1 {
		t.Fatalf("botsIndex = %d, want 1 after startup migration", botsIndex)
	}
	migrated := PattyGraph.matchers[0].asMatcher()
	if migrated == nil || migrated.matcherName() != "Googlebot" {
		t.Fatalf("first matcher = %v, want Googlebot matcher", PattyGraph.matchers[0].matcherName())
	}
	if migrated.lastIntervalCount != 1 {
		t.Fatalf("migrated lastIntervalCount = %d, want transferred pushed count 1", migrated.lastIntervalCount)
	}
	if migrated.intervalCount != 0 {
		t.Fatalf("migrated intervalCount after push = %d, want 0", migrated.intervalCount)
	}
}

func TestStartupPushCanAutoMigrateMultipleBotGroups(t *testing.T) {
	setupMonitorPipelineTestGraph()
	PattyGraph.botsMatcher.intervalCount = 19
	PattyGraph.botsMatcher.matchCountsMap["Googlebot"] = 10
	PattyGraph.botsMatcher.matchCountsMap["bingbot"] = 8
	PattyGraph.botsMatcher.matchCountsMap["Applebot"] = 1

	push()
	botsIndex = botsMatcherIndex()

	if botsMigrated != 2 {
		t.Fatalf("botsMigrated = %d, want 2", botsMigrated)
	}
	if botsIndex != 2 {
		t.Fatalf("botsIndex = %d, want 2 after two startup migrations", botsIndex)
	}
	if got := PattyGraph.matchers[0].matcherName(); got != "Googlebot" {
		t.Fatalf("first matcher = %q, want Googlebot", got)
	}
	if got := PattyGraph.matchers[1].matcherName(); got != "bingbot" {
		t.Fatalf("second matcher = %q, want bingbot", got)
	}
	if matcherNameExists("Applebot") {
		t.Fatal("Applebot should not migrate below startup threshold")
	}
	if PattyGraph.botsMatcher.lastIntervalCount != 1 {
		t.Fatalf("Bots lastIntervalCount = %d, want remaining Applebot count 1", PattyGraph.botsMatcher.lastIntervalCount)
	}
}

func TestBotsMigrationSkipsWhenHigherCompetingMatcherWins(t *testing.T) {
	setupMonitorPipelineTestGraph()
	winner := SimplePredicateMatcher("winner", []string{"winner"})
	winner.intervalCount = 3
	if !placeMatcher(winner, matcherBeforeBots) {
		t.Fatal("failed to place competing matcher")
	}

	PattyGraph.botsMatcher.intervalCount = 2
	PattyGraph.botsMatcher.matchCountsMap["Googlebot"] = 2
	PattyGraph.botsMatcher.migrateBots(0)

	if botsMigrated != 0 {
		t.Fatalf("botsMigrated = %d, want 0", botsMigrated)
	}
	if matcherNameExists("Googlebot") {
		t.Fatal("Googlebot matcher was migrated even though a higher competing matcher won")
	}
}
