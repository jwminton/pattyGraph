// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

func withCoverageArgs(t *testing.T, args ...string) {
	t.Helper()
	oldArgs := os.Args
	oldFlags := pflag.CommandLine
	os.Args = append([]string{"pattyGraph"}, args...)
	pflag.CommandLine = pflag.NewFlagSet("pattyGraph-test", pflag.ContinueOnError)
	t.Cleanup(func() {
		os.Args = oldArgs
		pflag.CommandLine = oldFlags
	})
}

func captureCoverageStdout(t *testing.T, fn func()) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stdout")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = f
	fn()
	os.Stdout = old
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestCoverageParseArgsAndCliEnforcement(t *testing.T) {
	resetRuntimeGlobalsForTest()
	dir := t.TempDir()
	withCoverageArgs(t,
		"--save-dir", dir,
		"--json-file", "traffic.jsonl",
		"--control-file", "commands.log",
		"--json-sources",
		"--read", "12",
		"--grace", "18",
		"--peak-limit", "17",
		"--flux", "4",
		"--push", "7",
		"--scale", "1.5",
		"--color-index", "3",
		"--title", "coverage",
		"--zero",
		"/tmp/access.log",
	)

	conf := parseArgs()
	if conf.filePath != "/tmp/access.log" || conf.mbToRead != 12 || conf.jsonFile != filepath.Join(dir, "traffic.jsonl") {
		t.Fatalf("unexpected parsed config: %#v", conf)
	}
	if !generateSidecarJSONL || !includeSidecarSourceExamples || !enableControlFile || !forceZeroStart {
		t.Fatal("boolean CLI settings were not applied")
	}

	PattyGraph = NewMonitor(conf)
	facts = NewFactoidGenerator()
	pattyGracePeriod = 1
	peakWordLimit = 1
	fluxDepth = 1
	pattyPushFactor = 1
	pattyScaleFactor = 1
	colorIndex = 0
	enforceCliFlags()
	if pattyGracePeriod != 18 || peakWordLimit != 17 || fluxDepth != 4 || pattyPushFactor != 7 || pattyScaleFactor != 1.5 || colorIndex != 3 {
		t.Fatal("explicit CLI settings were not re-applied")
	}
	if machineDisplayName != "coverage" || !generateSidecarJSONL || !enableControlFile {
		t.Fatal("CLI identity/output settings were not re-applied")
	}
}

func TestCoverageParseArgsDefaultsAndValidation(t *testing.T) {
	resetRuntimeGlobalsForTest()
	withCoverageArgs(t)
	conf := parseArgs()
	if conf.filePath != defaultLogFilename || conf.mbToRead != DefaultMBToRead {
		t.Fatalf("default CLI config changed: %#v", conf)
	}
}

func TestCoverageDumpTables(t *testing.T) {
	setupMonitorPipelineTestGraph()
	factsOutput := captureCoverageStdout(t, dumpFacts)
	if !strings.Contains(factsOutput, "settings.push") {
		t.Fatal("fact table omitted settings.push")
	}
	colorsOutput := captureCoverageStdout(t, dumpColors)
	if !strings.Contains(colorsOutput, "fuchsia") || !strings.Contains(colorsOutput, "yellow") {
		t.Fatal("color table omitted expected colors")
	}
}

func TestCoverageGeneratedUsageTopics(t *testing.T) {
	topics := []struct {
		name string
		want string
	}{
		{"colors", "Matcher colors"},
		{"facts", "Factoids"},
		{"inline", "Inline command reference"},
		{"layout", "Layout:"},
		{"words", "Built"},
		{"jsonl", "JSONL"},
		{"ai", "pattyGraph"},
		{"ordinary.log", "General Settings"},
	}
	for _, tc := range topics {
		t.Run(tc.name, func(t *testing.T) {
			resetRuntimeGlobalsForTest()
			withCoverageArgs(t, tc.name)
			_ = parseArgs()
			output := captureCoverageStdout(t, pflag.Usage)
			if !strings.Contains(output, tc.want) {
				t.Fatalf("usage topic %q omitted %q", tc.name, tc.want)
			}
		})
	}
}

func TestCoverageCleanStackTrace(t *testing.T) {
	stack := cleanStackTrace("github.com/jwminton/pattygraph")
	if stack == "" || !strings.Contains(stack, "Stack trace (filtered):") {
		t.Fatalf("clean stack omitted current test: %q", stack)
	}
	if got := cleanStackTrace("prefix-that-does-not-exist"); got != "Stack trace (filtered):\n" {
		t.Fatalf("unexpected stack lines for absent prefix: %q", got)
	}
}
