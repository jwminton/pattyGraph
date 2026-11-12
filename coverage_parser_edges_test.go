// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"strings"
	"testing"
)

func TestCoverageSecondPartOnly(t *testing.T) {
	line := `127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET / HTTP/1.1" 200 12 "-" "agent"`
	part, err := secondPartOnly(line)
	if err != nil || part == "" || part[0] != '"' {
		t.Fatalf("second part parse failed: %q %v", part, err)
	}
	if _, err := secondPartOnly("missing quotes"); err == nil {
		t.Fatal("missing quote was accepted")
	}
}

func TestCoverageParserRejections(t *testing.T) {
	validLine := `127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET / HTTP/1.1" 200 12 "-" "agent"`
	unterminatedAgent := strings.TrimSuffix(validLine, `"`)

	if _, err := findUserAgentCloseQuote(`"agent`, 0); err == nil {
		t.Fatal("unterminated user-agent was accepted")
	}

	*currentLine = lineSource{logLine: "missing quotes"}
	if err := splitLogLinePartsIntoCurrent(); err == nil {
		t.Fatal("current-line parser accepted a line without an opening quote")
	}

	*currentLine = lineSource{logLine: unterminatedAgent}
	if err := splitLogLinePartsIntoCurrent(); err == nil {
		t.Fatal("current-line parser accepted an unterminated user-agent")
	}

	if _, _, _, _, _, err := splitLogLineParts("missing quotes"); err == nil {
		t.Fatal("splitLogLineParts accepted a line without an opening quote")
	}
	if _, _, _, _, _, err := splitLogLineParts(`"GET / HTTP/1.1" 200 12 "-"`); err == nil {
		t.Fatal("splitLogLineParts accepted too few quoted fields")
	}
	if _, _, _, _, _, err := splitLogLineParts(unterminatedAgent); err == nil {
		t.Fatal("splitLogLineParts accepted an unterminated user-agent")
	}
}

func TestCoverageTimestampRejections(t *testing.T) {
	for _, input := range []string{"missing timestamp", "[short]"} {
		got, err := extractTimestamp(input)
		if err != nil {
			t.Fatalf("extractTimestamp(%q) error = %v, want nil", input, err)
		}
		if !got.IsZero() {
			t.Fatalf("extractTimestamp(%q) = %v, want zero time", input, got)
		}
	}

	if _, err := extractTimestamp("[99/Bad/9999:99:99:99] trailing"); err == nil {
		t.Fatal("extractTimestamp accepted an invalid timestamp")
	}
}
