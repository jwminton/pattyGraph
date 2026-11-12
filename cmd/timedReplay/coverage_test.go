// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func silenceReplayDiagnostics(t *testing.T) {
	t.Helper()
	old := os.Stderr
	f, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = f
	t.Cleanup(func() {
		os.Stderr = old
		_ = f.Close()
	})
}

func TestCoverageParseFlags(t *testing.T) {
	oldSet := flag.CommandLine
	oldArgs := os.Args
	flag.CommandLine = flag.NewFlagSet("timedReplay-test", flag.ContinueOnError)
	os.Args = []string{"timedReplay", "-file", "in.log", "-out", "out.log", "-speed", "2.5", "-sync0"}
	t.Cleanup(func() {
		flag.CommandLine = oldSet
		os.Args = oldArgs
	})

	conf := parseFlags()
	if conf.sourcePath != "in.log" || conf.outputPath != "out.log" || conf.speed != 2.5 || !conf.sync0 {
		t.Fatalf("unexpected parsed config: %#v", conf)
	}
}

func TestCoverageParseNginxLogTime(t *testing.T) {
	valid := `127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET / HTTP/1.1" 200 12`
	got, err := parseNginxLogTime(valid)
	if err != nil || got.Second() != 36 {
		t.Fatalf("valid timestamp failed: %v %v", got, err)
	}
	for _, line := range []string{"missing", "[unterminated", "[bad]"} {
		if _, err := parseNginxLogTime(line); err == nil {
			t.Fatalf("expected timestamp error for %q", line)
		}
	}
}

func TestCoverageReplayAndOutput(t *testing.T) {
	silenceReplayDiagnostics(t)
	input := strings.Join([]string{
		`bad line`,
		`127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /a HTTP/1.1" 200 12`,
		`127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /b HTTP/1.1" 200 13`,
		`127.0.0.1 - - [10/Oct/2000:13:55:37 -0700] "GET /c HTTP/1.1" 200 14`,
	}, "\n")
	var out bytes.Buffer
	if err := replay(strings.NewReader(input), &out, 0, false); err != nil {
		t.Fatal(err)
	}
	if strings.Count(out.String(), "\n") != 3 || strings.Contains(out.String(), "bad line") {
		t.Fatalf("unexpected replay output: %q", out.String())
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "out.log")
	w, closeOutput, err := openOutput(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, "first\n"); err != nil {
		t.Fatal(err)
	}
	if err := closeOutput(); err != nil {
		t.Fatal(err)
	}
	w, closeOutput, err = openOutput(path)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(w, "second\n")
	_ = closeOutput()
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "first\nsecond\n" {
		t.Fatalf("append output failed: %q %v", data, err)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestCoverageReplayErrorPaths(t *testing.T) {
	silenceReplayDiagnostics(t)
	line := `127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET / HTTP/1.1" 200 12`
	if err := replay(strings.NewReader(line), failingWriter{}, 0, false); err == nil {
		t.Fatal("expected replay write failure")
	}
	writer := bufio.NewWriter(failingWriter{})
	if err := writeGroup(writer, logGroup{lines: []string{"line"}}); err == nil {
		t.Fatal("expected group write failure")
	}

	didStart := false
	group := logGroup{second: time.Unix(36, 0), lines: []string{"a", "b"}}
	maybeStartReplay(false, &didStart, group)
	maybeStartReplay(false, &didStart, group)
	tracker := &progressTracker{}
	tracker.record(group)
	tracker.record(logGroup{second: group.second.Add(24 * time.Second), lines: []string{"c"}})
	if tracker.groupsWritten != 2 || tracker.linesWritten != 3 {
		t.Fatalf("unexpected progress: %#v", tracker)
	}
	waitForMatchingSecond(-1)
	waitForMatchingSecond(60)
	warnDropLine(7, errors.New("expected"))
}

func TestCoverageRunValidationAndFiles(t *testing.T) {
	silenceReplayDiagnostics(t)
	if err := run(replayConfig{speed: 0}); err == nil {
		t.Fatal("expected invalid speed rejection")
	}
	if err := run(replayConfig{sourcePath: filepath.Join(t.TempDir(), "missing"), speed: 1}); err == nil {
		t.Fatal("expected missing source rejection")
	}

	dir := t.TempDir()
	source := filepath.Join(dir, "source.log")
	output := filepath.Join(dir, "output.log")
	line := `127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET / HTTP/1.1" 200 12` + "\n"
	if err := os.WriteFile(source, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(replayConfig{sourcePath: source, outputPath: output, speed: 1_000_000}); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(output); err != nil || string(data) != line {
		t.Fatalf("run output mismatch: %q %v", data, err)
	}

	info, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := rejectReplayOutputOverlap(source, filepath.Join(dir, "new.log"), info); err != nil {
		t.Fatal(err)
	}
	if _, closeOutput, err := openOutput("-"); err != nil || closeOutput != nil {
		t.Fatalf("stdout output failed: %v", err)
	}
}
