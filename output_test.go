// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestWriteConfigEmitsUserMatcherCommandsButNotSystemAdds(t *testing.T) {
	setupMonitorPipelineTestGraph()
	invokeInlineCommand("!!! add googlebot")
	invokeInlineCommand("!!! add *range-watch --ips 192.0.")

	var buf bytes.Buffer
	writeConfig(&buf)
	out := buf.String()

	if !strings.Contains(out, "!!! add googlebot\n") {
		t.Fatalf("config did not contain googlebot add command:\n%s", out)
	}
	if !strings.Contains(out, "!!! add *range-watch --ips 192.0.\n") {
		t.Fatalf("config did not contain range-watch add command:\n%s", out)
	}
	for _, systemAdd := range []string{
		"!!! add lines",
		"!!! add bytes",
		"!!! add errs",
		"!!! add words",
		"!!! add refs",
		"!!! add ips",
	} {
		if strings.Contains(out, systemAdd) {
			t.Fatalf("config unexpectedly contained system add command %q:\n%s", systemAdd, out)
		}
	}
}

func TestWriteConfigEmitsActiveModeAndColorLines(t *testing.T) {
	setupMonitorPipelineTestGraph()
	invokeInlineCommand("!!! add googlebot")
	invokeInlineCommand("!!! mode googlebot 2")
	invokeInlineCommand("!!! color googlebot red")

	var buf bytes.Buffer
	writeConfig(&buf)
	out := buf.String()

	if !strings.Contains(out, "!!! mode googlebot 2\n") {
		t.Fatalf("config did not contain mode line:\n%s", out)
	}
	if !strings.Contains(out, "!!! color googlebot [red]\n") {
		t.Fatalf("config did not contain color line:\n%s", out)
	}
}

func TestPrintToFileStripsColorTags(t *testing.T) {
	setupMonitorPipelineTestGraph()
	dir := t.TempDir()
	PattyGraph.pattyConfig.saveDir = dir

	PattyGraph.sparklineHistoryView.SetText("[red]Spark[default]\n")
	PattyGraph.botMatchesView.SetText("[green]Bots[default]")
	PattyGraph.wordMatchesView.SetText("[blue]Words[default]")
	PattyGraph.refsView.SetText("[yellow]Refs[default]")
	PattyGraph.ipsView.SetText("[purple]Ips[default]")

	if err := PattyGraph.printToFile(); err != nil {
		t.Fatalf("printToFile() error = %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(dir, "pattySplat_*.txt"))
	if err != nil {
		t.Fatalf("glob error = %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("splat files = %v, want exactly one", matches)
	}
	if !regexp.MustCompile(`^pattySplat_[0-9]{8}_[0-9]{6}_[0-9]+\.txt$`).MatchString(filepath.Base(matches[0])) {
		t.Fatalf("splat file = %q, want shared timestamp stem", filepath.Base(matches[0]))
	}
	content, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	out := string(content)

	for _, tag := range []string{"[red]", "[green]", "[blue]", "[yellow]", "[purple]", "[default]"} {
		if strings.Contains(out, tag) {
			t.Fatalf("printToFile output still contained color tag %q:\n%s", tag, out)
		}
	}
	for _, text := range []string{"Spark", "Bots", "Words", "Refs", "Ips"} {
		if !strings.Contains(out, text) {
			t.Fatalf("printToFile output did not contain %q:\n%s", text, out)
		}
	}
}
