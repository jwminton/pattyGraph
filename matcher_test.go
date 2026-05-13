// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import "testing"

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

func TestInsertMatcherHelpersPreserveLayering(t *testing.T) {
	bots := NewPredicateMatcher("Bots")
	lines := NewPredicateMatcher("lines")
	PattyGraph = &Monitor{
		botsMatcher: bots,
		matchers:    []MatcherFacade{bots, lines},
	}

	top := NewPredicateMatcher("top")
	PattyGraph.matchers = insertMatcherFirst(PattyGraph.matchers, top)
	if PattyGraph.matchers[0] != top {
		t.Fatalf("insertMatcherFirst did not place matcher first")
	}
	if !top.isHistorical {
		t.Fatalf("top matcher isHistorical = false, want true")
	}

	beforeBots := NewPredicateMatcher("before-bots")
	PattyGraph.matchers = insertMatcherBeforeBots(PattyGraph.matchers, beforeBots)
	if PattyGraph.matchers[1] != beforeBots || PattyGraph.matchers[2] != bots {
		t.Fatalf("insertMatcherBeforeBots order = %s, %s; want before-bots, Bots", PattyGraph.matchers[1].matcherName(), PattyGraph.matchers[2].matcherName())
	}
	if !beforeBots.isHistorical {
		t.Fatalf("before-bots matcher isHistorical = false, want true")
	}

	beforeLines := NewPredicateMatcher("before-lines")
	PattyGraph.matchers = insertMatcherBeforeLines(PattyGraph.matchers, beforeLines)
	if PattyGraph.matchers[3] != beforeLines || PattyGraph.matchers[4] != lines {
		t.Fatalf("insertMatcherBeforeLines order = %s, %s; want before-lines, lines", PattyGraph.matchers[3].matcherName(), PattyGraph.matchers[4].matcherName())
	}
	if beforeLines.isHistorical {
		t.Fatalf("before-lines matcher isHistorical = true, want false")
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
