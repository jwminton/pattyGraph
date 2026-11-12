// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestCoverageInlineSettingBoundaries(t *testing.T) {
	setupMonitorPipelineTestGraph()
	for _, grace := range []int{-1, 2, 40, DefaultHistoryDepth + 10} {
		_ = setGracePeriod(grace)
	}
	if pattyGracePeriod != DefaultHistoryDepth+1 {
		t.Fatalf("grace upper clamp = %d", pattyGracePeriod)
	}
	if setGracePeriod(pattyGracePeriod) {
		t.Fatal("unchanged grace reported a change")
	}
	for _, flux := range []int{0, 1, 5, 10, 11} {
		_ = setFlux(flux)
	}
	for _, scale := range []float64{-1, 0.79, 2.35, 9} {
		setNewScaleFactor(scale)
	}
	if pattyScaleFactor != pattyBurstMax {
		t.Fatalf("scale upper clamp = %.1f", pattyScaleFactor)
	}

	PattyGraph.linesMatcher.color = "[beige]"
	if !colorInUse("[beige]") || colorInUse("[missing]") {
		t.Fatal("matcher color usage detection failed")
	}
	setMatcherMode("missing", 2)
	setMatcherMode("lines", 5)
	if PattyGraph.linesMatcher.displayMatchMode != 2 {
		t.Fatal("matcher mode was not normalized")
	}
}

func TestCoverageWriteConfigVariants(t *testing.T) {
	setupMonitorPipelineTestGraph()
	machineDisplayName = "coverage host"
	PattyGraph.pattyConfig.setSaveDir(t.TempDir())
	PattyGraph.pattyConfig.saveDirOriginal = "~/coverage"
	PattyGraph.pattyConfig.setJSONFile("traffic.jsonl")
	PattyGraph.pattyConfig.setControlFile("commands.log")
	includeSidecarSourceExamples = true
	generateSidecarJSONL = true
	enableControlFile = true
	PattyGraph.botsMatcher.disableAutoAdd = true
	PattyGraph.botsMatcher.isColorUserAssigned = true
	PattyGraph.botsMatcher.AlertAbove.set(10, "!!! alert Bots above 10")

	var out bytes.Buffer
	if err := writeConfig(&out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"title", "save-dir", "json-file", "json-sources on", "control-file", "alert Bots", "color Bots"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("generated config omitted %q:\n%s", want, out.String())
		}
	}

	PattyGraph.pattyConfig.jsonFileOriginal = ""
	PattyGraph.pattyConfig.jsonFile = ""
	PattyGraph.pattyConfig.controlFileOriginal = ""
	PattyGraph.pattyConfig.controlFile = ""
	out.Reset()
	if err := writeConfig(&out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "json on") || !strings.Contains(out.String(), "control on") {
		t.Fatalf("default output toggles omitted:\n%s", out.String())
	}
}

type coverageFailWriter struct{}

func (coverageFailWriter) Write([]byte) (int, error) { return 0, bytes.ErrTooLarge }

func TestCoverageWriteConfigFailure(t *testing.T) {
	setupMonitorPipelineTestGraph()
	machineDisplayName = "coverage"
	if err := writeConfig(coverageFailWriter{}); err == nil {
		t.Fatal("writeConfig accepted a failing writer")
	}
}
