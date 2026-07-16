// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSplitLogLinePartsIntoCurrentStandardNginxLine(t *testing.T) {
	*currentLine = lineSource{
		logLine: `192.0.2.10 - - [13/May/2026:14:22:31 -0700] "GET /robots.txt HTTP/1.1" 200 1234 "https://example.com/start" "Mozilla/5.0 Googlebot/2.1"`,
	}

	if err := splitLogLinePartsIntoCurrent(); err != nil {
		t.Fatalf("splitLogLinePartsIntoCurrent() error = %v", err)
	}

	if currentLine.request != "GET /robots.txt HTTP/1.1" {
		t.Fatalf("request = %q", currentLine.request)
	}
	if currentLine.respCode != "200" {
		t.Fatalf("respCode = %q", currentLine.respCode)
	}
	if currentLine.bytesValue != 1234 {
		t.Fatalf("bytesValue = %d", currentLine.bytesValue)
	}
	if currentLine.referer != "https://example.com/start" {
		t.Fatalf("referer = %q", currentLine.referer)
	}
	if currentLine.userAgent != "Mozilla/5.0 Googlebot/2.1" {
		t.Fatalf("userAgent = %q", currentLine.userAgent)
	}
}

func TestSplitLogLinePartsIntoCurrentEmptyUserAgent(t *testing.T) {
	*currentLine = lineSource{
		logLine: `198.51.100.7 - - [13/May/2026:14:22:31 -0700] "HEAD / HTTP/1.1" 204 0 "-" ""`,
	}

	if err := splitLogLinePartsIntoCurrent(); err != nil {
		t.Fatalf("splitLogLinePartsIntoCurrent() error = %v", err)
	}

	if currentLine.request != "HEAD / HTTP/1.1" {
		t.Fatalf("request = %q", currentLine.request)
	}
	if currentLine.respCode != "204" {
		t.Fatalf("respCode = %q", currentLine.respCode)
	}
	if currentLine.bytesValue != 0 {
		t.Fatalf("bytesValue = %d", currentLine.bytesValue)
	}
	if currentLine.referer != "-" {
		t.Fatalf("referer = %q", currentLine.referer)
	}
	if currentLine.userAgent != "" {
		t.Fatalf("userAgent = %q", currentLine.userAgent)
	}
}

func TestSplitLogLinePartsIntoCurrentIgnoresTrailingQuotedField(t *testing.T) {
	*currentLine = lineSource{
		logLine: `192.0.2.10 - - [13/May/2026:14:22:31 -0700] "GET /admin HTTP/1.1" 200 1234 "-" "SuspiciousBot" "-"`,
	}

	if err := splitLogLinePartsIntoCurrent(); err != nil {
		t.Fatalf("splitLogLinePartsIntoCurrent() error = %v", err)
	}

	if currentLine.userAgent != "SuspiciousBot" {
		t.Fatalf("userAgent = %q, want SuspiciousBot", currentLine.userAgent)
	}
}

func TestSplitLogLinePartsIntoCurrentIgnoresTrailingSpace(t *testing.T) {
	*currentLine = lineSource{
		logLine: `192.0.2.10 - - [13/May/2026:14:22:31 -0700] "GET /admin HTTP/1.1" 200 1234 "-" "SuspiciousBot" `,
	}

	if err := splitLogLinePartsIntoCurrent(); err != nil {
		t.Fatalf("splitLogLinePartsIntoCurrent() error = %v", err)
	}

	if currentLine.userAgent != "SuspiciousBot" {
		t.Fatalf("userAgent = %q, want SuspiciousBot", currentLine.userAgent)
	}
}

func TestSplitLogLinePartsIntoCurrentAllowsQuotesInsideUserAgent(t *testing.T) {
	*currentLine = lineSource{
		logLine: `192.0.2.10 - - [13/May/2026:14:22:31 -0700] "GET /admin HTTP/1.1" 200 1234 "-" "Suspicious "Quoted" Bot"`,
	}

	if err := splitLogLinePartsIntoCurrent(); err != nil {
		t.Fatalf("splitLogLinePartsIntoCurrent() error = %v", err)
	}

	if currentLine.userAgent != `Suspicious "Quoted" Bot` {
		t.Fatalf("userAgent = %q, want quoted content preserved", currentLine.userAgent)
	}
}

func TestSplitLogLinePartsIntoCurrentBadByteCount(t *testing.T) {
	*currentLine = lineSource{
		logLine: `203.0.113.9 - - [13/May/2026:14:22:31 -0700] "GET / HTTP/1.1" 200 - "-" "curl/8.0"`,
	}

	if err := splitLogLinePartsIntoCurrent(); err == nil {
		t.Fatal("splitLogLinePartsIntoCurrent() error = nil, want error")
	}
}

func TestSplitLogLinePartsIntoCurrentNotEnoughQuotes(t *testing.T) {
	*currentLine = lineSource{
		logLine: `203.0.113.9 - - [13/May/2026:14:22:31 -0700] "GET / HTTP/1.1" 200 12 "-"`,
	}

	if err := splitLogLinePartsIntoCurrent(); err == nil {
		t.Fatal("splitLogLinePartsIntoCurrent() error = nil, want error")
	}
}

func TestParseNginxTimeFast(t *testing.T) {
	line := `192.0.2.10 - - [13/May/2026:14:22:31 -0700] "GET / HTTP/1.1" 200 12 "-" "curl/8.0"`

	got, err := parseNginxTimeFast(line)
	if err != nil {
		t.Fatalf("parseNginxTimeFast() error = %v", err)
	}

	want := time.Date(2026, time.May, 13, 14, 22, 31, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("parseNginxTimeFast() = %s, want %s", got, want)
	}
}

func TestParseNginxTimeFastInvalidMonth(t *testing.T) {
	line := `192.0.2.10 - - [13/Wat/2026:14:22:31 -0700] "GET / HTTP/1.1" 200 12 "-" "curl/8.0"`

	if _, err := parseNginxTimeFast(line); err == nil {
		t.Fatal("parseNginxTimeFast() error = nil, want error")
	}
}

func TestPreloadGroupingIgnoresHistoricalInlineCommands(t *testing.T) {
	path := filepath.Join(t.TempDir(), "access.log")
	lines := []string{
		`192.0.2.10 - - [13/May/2026:14:22:31 -0700] "GET /before HTTP/1.1" 200 12 "-" "curl/8.0"`,
		`!!! fact print historical marker # ignored during preload`,
		`192.0.2.11 - - [13/May/2026:14:22:45 -0700] "GET /after HTTP/1.1" 200 13 "-" "curl/8.0"`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	groups, err := groupLinesByMinuteInMb(path, 1)
	if err != nil {
		t.Fatalf("groupLinesByMinuteInMb: %v", err)
	}
	if len(groups) != 1 || len(groups[0].Lines) != 2 {
		t.Fatalf("groups = %#v, want one minute containing two NGINX lines", groups)
	}
	for _, line := range groups[0].Lines {
		if strings.HasPrefix(line, InlinePreamble) {
			t.Fatalf("historical inline command entered preload group: %q", line)
		}
	}
}

func TestIsLikelyIPv4AndPrefix(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantOK     bool
		wantPrefix string
	}{
		{name: "full ip", input: "192.0.2.10", wantOK: true, wantPrefix: "192.0."},
		{name: "long final octet accepted", input: "203.0.113.999", wantOK: true, wantPrefix: "203.0."},
		{name: "hostname rejected", input: "example.com", wantOK: false},
		{name: "missing third octet digit rejected", input: "192.0.x.1", wantOK: false},
		{name: "not enough dots rejected", input: "192.0", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOK, gotPrefix := isLikelyIPv4AndPrefix(tt.input)
			if gotOK != tt.wantOK {
				t.Fatalf("ok = %v, want %v", gotOK, tt.wantOK)
			}
			if gotPrefix != tt.wantPrefix {
				t.Fatalf("prefix = %q, want %q", gotPrefix, tt.wantPrefix)
			}
		})
	}
}
