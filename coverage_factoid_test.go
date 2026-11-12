// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"strings"
	"testing"
)

func TestCoverageFactoidSchedules(t *testing.T) {
	if !CycleSchedule(3, 1)(4, 0, false) || CycleSchedule(3, 1)(3, 0, false) {
		t.Fatal("cycle schedule mismatch")
	}
	if RandomSchedule(0)(20, 0, false) || !RandomSchedule(100)(20, 0, false) {
		t.Fatal("random schedule bounds mismatch")
	}
	if RandomSchedule(100)(20, 15, false) {
		t.Fatal("random cooldown was ignored")
	}
	if RepeatingSchedule(100)(20, 19, false) || !RepeatingSchedule(100)(20, 18, false) {
		t.Fatal("repeating cooldown mismatch")
	}
	if !OnceSchedule()(0, 0, false) || OnceSchedule()(0, 0, true) {
		t.Fatal("once schedule mismatch")
	}
	if EverySchedule(5)(4, 0, false) || !EverySchedule(5)(5, 0, false) {
		t.Fatal("every schedule mismatch")
	}
	if !AlwaysSchedule()(0, 0, false) || DirectOnlySchedule()(0, 0, false) {
		t.Fatal("fixed schedule mismatch")
	}

	g := &FactoidGenerator{}
	gen := func(_ []string) string { return "value" }
	g.Add(CycleMod(2, 0, gen), "cycle")
	g.Add(Every(2, gen), "every")
	g.Add(Always(gen), "always")
	g.Add(DirectOnly(gen), "direct")
	if len(g.facts) != 4 {
		t.Fatalf("unexpected fact count: %d", len(g.facts))
	}
}

func TestCoverageFactoidCatalogAndPanels(t *testing.T) {
	setupMonitorPipelineTestGraph()
	if facts == nil || len(facts.facts) < 20 {
		t.Fatalf("factoid catalog unexpectedly small: %d", len(facts.facts))
	}
	for _, name := range []string{"settings.push", "settings.pressure", "version.pattygraph", "output.paths"} {
		if factoidByName[name] == nil {
			t.Fatalf("missing named factoid %q", name)
		}
	}

	bottomPanelCurrent = bottomPanelMatchers
	bottomPanelReturnMode = bottomPanelMatchers
	syncBottomPanelState()
	if showMetricsPanelContents || tickerPreamble != defaultTickerBg {
		t.Fatal("matcher panel state was not synchronized")
	}
	togglePreamble()
	if bottomPanelCurrent != bottomPanelFactoids || !showMetricsPanelContents {
		t.Fatal("factoid panel did not activate")
	}
	toggleHelpPanel()
	if bottomPanelCurrent != bottomPanelHelp || tickerPreamble != selectedTickerBg {
		t.Fatal("help panel did not retain factoid context")
	}
	toggleHelpPanel()
	if bottomPanelCurrent != bottomPanelFactoids {
		t.Fatal("help panel did not restore prior mode")
	}
	togglePreamble()

	if !strings.Contains(quickHelpPanelContents(), "Quick Help") {
		t.Fatal("quick help heading missing")
	}
	factoidHistory = []string{"one", "two"}
	panel := metricPanelContents()
	if !strings.Contains(panel, "one\ntwo") || !strings.Contains(panel, "[default:black]") {
		t.Fatal("metric panel omitted expected content")
	}
	if !strings.Contains(memFmt("value %d"), memGcColor) || !strings.Contains(ipFmt("%s"), ipColor) {
		t.Fatal("factoid formatter omitted color")
	}
	if avgBytes() == "" || avgLines() == "" {
		t.Fatal("factoid averages returned empty text")
	}
}

func TestCoverageFactoidGenerationPaths(t *testing.T) {
	setupMonitorPipelineTestGraph()
	doRandomFact = true

	g := &FactoidGenerator{}
	empty := Always(func(_ []string) string { return "" })
	value := Always(func(args []string) string { return "generated:" + strings.Join(args, ",") })
	g.Add(empty, "empty")
	g.Add(value, "value")
	value.args = []string{"x"}

	for i := 0; i < 20; i++ {
		text, _, name := g.NextBackground()
		if text != "   " {
			if name != "value" || !strings.HasPrefix(text, "generated:") {
				t.Fatalf("unexpected generated factoid: %q %q", name, text)
			}
			break
		}
	}

	value.args = []string{"forced"}
	g.forced = append(g.forced, value)
	text, rank, name := g.Next()
	if text != "generated:forced" || rank != value.probability || name != "value" {
		t.Fatalf("unexpected forced factoid: %q %d %q", text, rank, name)
	}
}
