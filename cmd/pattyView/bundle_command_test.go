package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pattyGraph/cmd/pattyView/internal/investigation"
)

const (
	bundleLogTime0 = "2026-08-08T08:01:00-07:00"
	bundleLogTime1 = "2026-08-08T08:02:00-07:00"
)

func TestParseApplicationOptionsBuildsBundleMode(t *testing.T) {
	options, err := parseApplicationOptions([]string{
		"--bundle", "traffic.jsonl",
		"--from", bundleLogTime0,
		"--through", bundleLogTime1,
		"--session", "session-a",
		"--output", "incident.zip",
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parse bundle options: %v", err)
	}
	if options.bundle == nil {
		t.Fatal("bundle mode was not selected")
	}
	if options.bundle.inputPath != "traffic.jsonl" || options.bundle.outputPath != "incident.zip" {
		t.Fatalf("bundle paths = %#v", options.bundle)
	}
	if options.bundle.sessionID != "session-a" {
		t.Fatalf("bundle session = %q", options.bundle.sessionID)
	}
}

func TestParseBundleOptionsAddsMissingZipSuffix(t *testing.T) {
	tests := []struct {
		output string
		want   string
	}{
		{output: "incident", want: "incident.zip"},
		{output: "incident.zip", want: "incident.zip"},
		{output: "incident.ZIP", want: "incident.ZIP"},
	}
	for _, test := range tests {
		options, err := parseBundleOptions(
			"traffic.jsonl",
			bundleLogTime0,
			bundleLogTime1,
			"session-a",
			test.output,
		)
		if err != nil {
			t.Fatalf("parse output %q: %v", test.output, err)
		}
		if options.outputPath != test.want {
			t.Errorf("output path = %q, want %q", options.outputPath, test.want)
		}
	}
}

func TestParseApplicationOptionsRejectsInvalidBundleCombinations(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "orphan range", args: []string{"--from", bundleLogTime0}, want: "require --bundle"},
		{name: "missing input", args: []string{"--bundle=", "--from", bundleLogTime0, "--through", bundleLogTime1}, want: "requires a PattyLog"},
		{name: "missing from", args: []string{"--bundle", "traffic.jsonl", "--through", bundleLogTime1}, want: "requires --from"},
		{name: "missing through", args: []string{"--bundle", "traffic.jsonl", "--from", bundleLogTime0}, want: "requires --through"},
		{name: "invalid time", args: []string{"--bundle", "traffic.jsonl", "--from", "today", "--through", bundleLogTime1}, want: "invalid --from"},
		{name: "reversed", args: []string{"--bundle", "traffic.jsonl", "--from", bundleLogTime1, "--through", bundleLogTime0}, want: "is after"},
		{name: "listen conflict", args: []string{"--bundle", "traffic.jsonl", "--from", bundleLogTime0, "--through", bundleLogTime1, "--listen", "127.0.0.1:0"}, want: "cannot be combined with --listen"},
		{name: "version conflict", args: []string{"--bundle", "traffic.jsonl", "--from", bundleLogTime0, "--through", bundleLogTime1, "--version"}, want: "cannot be combined with --version"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseApplicationOptions(test.args, &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("parse error = %v, want text %q", err, test.want)
			}
		})
	}
}

func TestRunCreatesDefaultIncidentBundleForSoleSession(t *testing.T) {
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "pattyLog_sample.jsonl")
	inputContents := testSession("sole", bundleLogTime0, bundleLogTime1) + "\n"
	writeTestPattyLog(t, inputPath, strings.TrimSuffix(inputContents, "\n"))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--bundle", inputPath, "--from", bundleLogTime0, "--through", bundleLogTime1}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("create incident bundle: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("bundle stderr = %q", stderr.String())
	}
	expectedPath := defaultBundleOutputPath(inputPath, mustBundleTime(bundleLogTime0), mustBundleTime(bundleLogTime1))
	if !strings.Contains(stdout.String(), expectedPath) || !strings.Contains(stdout.String(), "session sole") {
		t.Fatalf("bundle success output = %q", stdout.String())
	}
	info, err := os.Stat(expectedPath)
	if err != nil {
		t.Fatalf("stat incident bundle: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("bundle mode = %o, want 600", got)
	}

	archive, err := zip.OpenReader(expectedPath)
	if err != nil {
		t.Fatalf("open incident bundle: %v", err)
	}
	defer archive.Close()
	if len(archive.File) != 2 || archive.File[0].Name != investigation.ManifestEntryName || archive.File[1].Name != investigation.PattyLogEntryName {
		t.Fatalf("bundle entries = %#v", archive.File)
	}
	manifestReader, err := archive.File[0].Open()
	if err != nil {
		t.Fatalf("open bundle manifest: %v", err)
	}
	defer manifestReader.Close()
	var manifest investigation.Manifest
	if err := json.NewDecoder(manifestReader).Decode(&manifest); err != nil {
		t.Fatalf("decode bundle manifest: %v", err)
	}
	if manifest.PattyLog.SessionID != "sole" || manifest.PattyLog.Representation != "source" || manifest.Creator.Version != PattyViewVersion {
		t.Fatalf("bundle manifest = %#v", manifest)
	}
	unchanged, err := os.ReadFile(inputPath)
	if err != nil || string(unchanged) != inputContents {
		t.Fatalf("source PattyLog changed: %v", err)
	}
}

func TestDefaultBundleOutputPathUsesCompactHumanName(t *testing.T) {
	from := mustBundleTime("2026-08-09T14:20:00-07:00")
	through := mustBundleTime("2026-08-09T15:10:59-07:00")
	if got, want := filepath.Base(defaultBundleOutputPath("traffic.incident.zip", from, through)),
		"traffic_20260809_1420-1510.incident.zip"; got != want {
		t.Fatalf("same-day bundle name = %q, want %q", got, want)
	}

	crossDay := mustBundleTime("2026-08-10T00:10:00-07:00")
	if got, want := filepath.Base(defaultBundleOutputPath("traffic.jsonl", from, crossDay)),
		"traffic_20260809_1420-20260810_0010.incident.zip"; got != want {
		t.Fatalf("cross-day bundle name = %q, want %q", got, want)
	}

	longStem := strings.Repeat("é", 40) + ".jsonl"
	if got, want := filepath.Base(defaultBundleOutputPath(longStem, from, through)),
		strings.Repeat("é", 32)+"_20260809_1420-1510.incident.zip"; got != want {
		t.Fatalf("long-stem bundle name = %q, want %q", got, want)
	}
}

func TestRunReportsInputAndOutputFilesystemErrors(t *testing.T) {
	directory := t.TempDir()
	missingInput := filepath.Join(directory, "missing.jsonl")
	err := run([]string{"--bundle", missingInput, "--from", bundleLogTime0, "--through", bundleLogTime1}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "open PattyLog") {
		t.Fatalf("missing input error = %v", err)
	}

	inputPath := filepath.Join(directory, "source.jsonl")
	writeTestPattyLog(t, inputPath, testSession("filesystem", bundleLogTime0, bundleLogTime1))
	missingOutput := filepath.Join(directory, "missing", "incident.zip")
	err = run([]string{"--bundle", inputPath, "--from", bundleLogTime0, "--through", bundleLogTime1, "--output", missingOutput}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "create incident bundle") {
		t.Fatalf("missing output directory error = %v", err)
	}
	if _, statErr := os.Stat(filepath.Dir(missingOutput)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("bundle command created missing output directory: %v", statErr)
	}
}

func TestRunRequiresSessionForMultiSessionPattyLog(t *testing.T) {
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "multi.jsonl")
	writeTestPattyLog(t, inputPath, testSession("first", bundleLogTime0, bundleLogTime1)+"\n"+testSession("second", bundleLogTime0, bundleLogTime1))

	var stdout bytes.Buffer
	err := run([]string{"--bundle", inputPath, "--from", bundleLogTime0, "--through", bundleLogTime1}, &stdout, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "select one of first, second") {
		t.Fatalf("multi-session error = %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("failed bundle stdout = %q", stdout.String())
	}
	if _, statErr := os.Stat(defaultBundleOutputPath(inputPath, mustBundleTime(bundleLogTime0), mustBundleTime(bundleLogTime1))); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("ambiguous bundle created output: %v", statErr)
	}

	outputPath := filepath.Join(directory, "second.zip")
	err = run([]string{"--bundle", inputPath, "--from", bundleLogTime0, "--through", bundleLogTime1, "--session", "second", "--output", outputPath}, &stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("create explicit-session bundle: %v", err)
	}
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("explicit-session bundle missing: %v", err)
	}
}

func TestRunRefusesToOverwriteIncidentBundle(t *testing.T) {
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "source.jsonl")
	outputPath := filepath.Join(directory, "existing.zip")
	writeTestPattyLog(t, inputPath, testSession("existing", bundleLogTime0, bundleLogTime1))
	if err := os.WriteFile(outputPath, []byte("keep"), 0o600); err != nil {
		t.Fatalf("write existing output: %v", err)
	}

	err := run([]string{"--bundle", inputPath, "--from", bundleLogTime0, "--through", bundleLogTime1, "--output", outputPath}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("overwrite error = %v", err)
	}
	contents, readErr := os.ReadFile(outputPath)
	if readErr != nil || string(contents) != "keep" {
		t.Fatalf("existing output changed: %q, %v", contents, readErr)
	}
}

func TestWriteBundleFileRemovesIncompleteOutput(t *testing.T) {
	input := testSession("changed", bundleLogTime0, bundleLogTime1)
	reader := strings.NewReader(input)
	plan, err := investigation.PlanSelection(reader, investigation.SelectionRequest{
		SessionID:      "changed",
		FromLogTime:    mustBundleTime(bundleLogTime0),
		ThroughLogTime: mustBundleTime(bundleLogTime1),
		SourceName:     "changed.jsonl",
		CreatorVersion: PattyViewVersion,
	})
	if err != nil {
		t.Fatalf("plan changed input: %v", err)
	}
	outputPath := filepath.Join(t.TempDir(), "incomplete.zip")
	err = writeBundleFile(outputPath, strings.NewReader(strings.Split(input, "\n")[0]), plan)
	if err == nil {
		t.Fatal("changed input did not fail bundle creation")
	}
	if _, statErr := os.Stat(outputPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("incomplete bundle remains: %v", statErr)
	}
}

func testSession(sessionID, first, second string) string {
	return strings.Join([]string{
		fmtJSON(map[string]any{"schema_version": 4, "event_type": "session_start", "session_id": sessionID, "log_time": "1970-01-01T00:00:00Z"}),
		fmtJSON(map[string]any{"schema_version": 4, "event_type": "interval", "session_id": sessionID, "interval": 0, "log_time": first, "interval_lines": 10}),
		fmtJSON(map[string]any{"schema_version": 4, "event_type": "interval", "session_id": sessionID, "interval": 1, "log_time": second, "interval_lines": 12}),
	}, "\n")
}

func fmtJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func writeTestPattyLog(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents+"\n"), 0o600); err != nil {
		t.Fatalf("write test PattyLog: %v", err)
	}
}

func mustBundleTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		panic(err)
	}
	return parsed
}
