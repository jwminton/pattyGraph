// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestControlFilePathUsesCurrentDirectoryWhenSaveDirEmpty(t *testing.T) {
	setupMonitorPipelineTestGraph()

	if got := controlFilePath(); got != "pattyControl.log" {
		t.Fatalf("controlFilePath() = %q, want pattyControl.log", got)
	}
}

func TestControlFilePathUsesSaveDirWhenPresent(t *testing.T) {
	setupMonitorPipelineTestGraph()
	PattyGraph.pattyConfig.saveDir = "/tmp/patty"

	want := filepath.Join("/tmp/patty", "pattyControl.log")
	if got := controlFilePath(); got != want {
		t.Fatalf("controlFilePath() = %q, want %q", got, want)
	}
}

func TestParseControlEnabled(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{value: "on", want: true},
		{value: "true", want: true},
		{value: "1", want: true},
		{value: "yes", want: true},
		{value: "unknown", want: true},
		{value: "", want: true},
		{value: "off", want: false},
		{value: "false", want: false},
		{value: "0", want: false},
		{value: "no", want: false},
		{value: " OFF ", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			if got := parseControlEnabled(tt.value); got != tt.want {
				t.Fatalf("parseControlEnabled(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestIsControlEnableLine(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{line: "!!! control", want: true},
		{line: "!!! control on", want: true},
		{line: "!!! control true", want: true},
		{line: "!!! control 1", want: true},
		{line: "!!! control off", want: false},
		{line: "!!! control false", want: false},
		{line: "!!! control 0", want: false},
		{line: "!!! add googlebot", want: false},
		{line: "control on", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			if got := isControlEnableLine(tt.line); got != tt.want {
				t.Fatalf("isControlEnableLine(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

func TestShouldProcessControlLineGatesCommands(t *testing.T) {
	setupMonitorPipelineTestGraph()
	enableControlFile = true

	if !shouldProcessControlLine("!!! add googlebot") {
		t.Fatal("enabled control file did not process normal inline command")
	}
	if shouldProcessControlLine("not an inline command") {
		t.Fatal("non-inline control line was processed")
	}

	enableControlFile = false
	if shouldProcessControlLine("!!! add googlebot") {
		t.Fatal("disabled control file processed normal inline command")
	}
	if !shouldProcessControlLine("!!! control") {
		t.Fatal("disabled control file did not process bare enable command")
	}
	if !shouldProcessControlLine("!!! control on") {
		t.Fatal("disabled control file did not process control on")
	}
	if shouldProcessControlLine("!!! control off") {
		t.Fatal("disabled control file processed control off")
	}
}

func TestControlFileStartMarkerIsCommentAndIgnoredAsCommand(t *testing.T) {
	setupMonitorPipelineTestGraph()
	enableControlFile = true
	PattyGraph.filePath = "./access.log"
	PattyGraph.pattyConfig.saveDir = "./splats"

	marker := controlFileStartMarker()

	if !strings.HasPrefix(marker, "# ") {
		t.Fatalf("marker = %q, want comment line", marker)
	}
	if shouldProcessControlLine(marker) {
		t.Fatalf("marker %q was treated as an inline command", marker)
	}
	wantParts := []string{
		"pattyGraph control ready",
		"session_id=" + sidecarSessionID,
		"control_file_enabled=true",
		"file_path=\"./access.log\"",
		"sidecar_path=\"splats/pattyLog_",
	}
	for _, want := range wantParts {
		if !strings.Contains(marker, want) {
			t.Fatalf("marker %q does not contain %q", marker, want)
		}
	}
}
