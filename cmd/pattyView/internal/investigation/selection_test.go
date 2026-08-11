package investigation

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

const (
	logTime0 = "2026-08-08T08:01:00-07:00"
	logTime1 = "2026-08-08T08:02:00-07:00"
	logTime2 = "2026-08-08T08:03:00-07:00"
	logTime3 = "2026-08-08T08:04:00-07:00"
)

func TestPlanSelectionPreservesIntervalContextAndRawRecords(t *testing.T) {
	input := strings.Join([]string{
		`{"schema_version":4,"event_type":"session_start","session_id":"incident","log_time":"1970-01-01T00:00:00Z"}`,
		intervalJSON("incident", 0, logTime0, nil),
		`{"schema_version":4,"event_type":"control_command","session_id":"incident","log_time":"2026-08-08T08:01:20-07:00","result":{"fact":"print","text":"Note: deploy"}}`,
		`{malformed but retained for the selected interval}`,
		`{"schema_version":4,"event_type":"alert","session_id":"incident","interval":0,"log_time":"2026-08-08T08:01:30-07:00"}`,
		`{"schema_version":7,"event_type":"future_event","session_id":"incident","log_time":"2026-08-08T08:01:40-07:00"}`,
		intervalJSON("incident", 1, logTime1, []string{"GET /checkout HTTP/1.1"}),
		`{"schema_version":4,"event_type":"alert","session_id":"incident","interval":1,"log_time":"2026-08-08T08:02:10-07:00"}`,
		`{"schema_version":4,"event_type":"control_command","session_id":"incident","log_time":"2026-08-08T08:02:20-07:00","command_name":"color"}`,
		`{"schema_version":7,"event_type":"future_event","session_id":"incident","log_time":"2026-08-08T08:02:30-07:00"}`,
		intervalJSON("incident", 2, logTime2, nil),
		`{"schema_version":4,"event_type":"alert","session_id":"incident","interval":2,"log_time":"2026-08-08T08:03:10-07:00"}`,
		`{"schema_version":4,"event_type":"control_command","session_id":"incident","log_time":"2026-08-08T08:03:20-07:00","result":{"fact":"print","text":"Note: outside"}}`,
		intervalJSON("incident", 3, logTime3, nil),
	}, "\r\n")

	reader := strings.NewReader(input)
	plan, err := PlanSelection(reader, selectionRequest("incident", logTime1, logTime2))
	if err != nil {
		t.Fatalf("plan selection: %v", err)
	}
	if got, want := plan.Manifest.Range.IntervalCount, 2; got != want {
		t.Fatalf("interval count = %d, want %d", got, want)
	}
	if got, want := plan.Manifest.PattyLog.RecordCount, 10; got != want {
		t.Fatalf("record count = %d, want %d", got, want)
	}
	if got, want := plan.Manifest.PattyLog.RetainedSourceIntervals, 1; got != want {
		t.Fatalf("retained source intervals = %d, want %d", got, want)
	}
	if got, want := plan.Manifest.PattyLog.SchemaVersions, []int{4, 7}; !equalInts(got, want) {
		t.Fatalf("schema versions = %v, want %v", got, want)
	}
	if got, want := plan.Manifest.PattyLog.SourceName, "source.jsonl"; got != want {
		t.Fatalf("source name = %q, want %q", got, want)
	}
	if plan.Manifest.Range.FromInterval != 1 || plan.Manifest.Range.ThroughInterval != 2 {
		t.Fatalf("range intervals = %d through %d", plan.Manifest.Range.FromInterval, plan.Manifest.Range.ThroughInterval)
	}

	var output bytes.Buffer
	if err := plan.WritePattyLog(reader, &output); err != nil {
		t.Fatalf("write selection: %v", err)
	}
	if strings.Contains(output.String(), `"interval":0`) || strings.Contains(output.String(), `"interval":3`) {
		t.Fatalf("output includes an unselected interval:\n%s", output.String())
	}
	for _, wanted := range []string{
		`"event_type":"session_start"`,
		`Note: deploy`,
		`{malformed but retained for the selected interval}`,
		`"schema_version":7,"event_type":"future_event"`,
		`"interval":1`,
		`"command_name":"color"`,
		`"interval":2`,
		`"interval":2,"log_time":"2026-08-08T08:03:10-07:00"`,
	} {
		if !strings.Contains(output.String(), wanted) {
			t.Errorf("output does not contain %q:\n%s", wanted, output.String())
		}
	}
	for _, unwanted := range []string{`"interval":0,"log_time":"2026-08-08T08:01:30-07:00"`, `Note: outside`} {
		if strings.Contains(output.String(), unwanted) {
			t.Errorf("output unexpectedly contains %q:\n%s", unwanted, output.String())
		}
	}
	if strings.Contains(output.String(), "\r") {
		t.Fatal("output retained CR line endings")
	}
	if got, want := strings.Count(output.String(), "\n"), plan.Manifest.PattyLog.RecordCount; got != want {
		t.Fatalf("output line count = %d, want %d", got, want)
	}
}

func TestPlanSelectionUsesFileOrderAcrossIntermediateClockSkew(t *testing.T) {
	input := pattyLog(
		intervalJSON("skew", 0, logTime0, nil),
		intervalJSON("skew", 1, logTime2, nil),
		intervalJSON("skew", 2, logTime1, nil),
		intervalJSON("skew", 3, logTime3, nil),
	)
	plan, err := PlanSelection(strings.NewReader(input), selectionRequest("skew", logTime2, logTime3))
	if err != nil {
		t.Fatalf("plan skewed selection: %v", err)
	}
	if got, want := plan.Manifest.Range.IntervalCount, 3; got != want {
		t.Fatalf("interval count = %d, want %d", got, want)
	}

	var output bytes.Buffer
	reader := strings.NewReader(input)
	if err := plan.WritePattyLog(reader, &output); err != nil {
		t.Fatalf("write skewed selection: %v", err)
	}
	for _, interval := range []string{`"interval":1`, `"interval":2`, `"interval":3`} {
		if !strings.Contains(output.String(), interval) {
			t.Errorf("skewed output does not contain %s", interval)
		}
	}
}

func TestPlanSelectionFallsBackToTimestamp(t *testing.T) {
	input := strings.Join([]string{
		`{"schema_version":4,"event_type":"session_start","session_id":"fallback","timestamp":"1970-01-01T00:00:00Z"}`,
		`{"schema_version":4,"event_type":"interval","session_id":"fallback","timestamp":"2026-08-08T08:01:00-07:00","interval":0}`,
	}, "\n")
	plan, err := PlanSelection(strings.NewReader(input), selectionRequest("fallback", logTime0, logTime0))
	if err != nil {
		t.Fatalf("plan timestamp fallback: %v", err)
	}
	if got := plan.Manifest.Range.FromLogTime; got != logTime0 {
		t.Fatalf("manifest from log-time = %q, want %q", got, logTime0)
	}
}

func TestPlanSelectionInfersSoleSession(t *testing.T) {
	request := selectionRequest("", logTime0, logTime1)
	plan, err := PlanSelection(strings.NewReader(pattyLog(
		intervalJSON("inferred", 0, logTime0, nil),
		intervalJSON("inferred", 1, logTime1, nil),
	)), request)
	if err != nil {
		t.Fatalf("infer sole session: %v", err)
	}
	if got, want := plan.Manifest.PattyLog.SessionID, "inferred"; got != want {
		t.Fatalf("inferred session = %q, want %q", got, want)
	}
}

func TestPlanSelectionRejectsAmbiguousSessionInference(t *testing.T) {
	input := strings.Join([]string{
		pattyLog(intervalJSON("z-session", 0, logTime0, nil)),
		pattyLog(intervalJSON("a-session", 0, logTime0, nil)),
	}, "\n")
	_, err := PlanSelection(strings.NewReader(input), selectionRequest("", logTime0, logTime0))
	assertSelectionError(t, err, ErrorAmbiguousSession)
	if !strings.Contains(err.Error(), "a-session, z-session") {
		t.Fatalf("ambiguous session error does not list sorted choices: %v", err)
	}
}

func TestManifestJSONIsStableAndContainsNoWallClock(t *testing.T) {
	input := pattyLog(intervalJSON("manifest", 0, logTime0, nil))
	plan, err := PlanSelection(strings.NewReader(input), selectionRequest("manifest", logTime0, logTime0))
	if err != nil {
		t.Fatalf("plan selection: %v", err)
	}
	encoded, err := json.Marshal(plan.Manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	text := string(encoded)
	for _, wanted := range []string{
		`"bundle_schema":1`,
		`"bundle_type":"pattygraph_incident"`,
		`"entry":"pattyLog.jsonl"`,
		`"creator":{"name":"pattyView","version":"0.1.8"}`,
	} {
		if !strings.Contains(text, wanted) {
			t.Errorf("manifest does not contain %s: %s", wanted, text)
		}
	}
	if strings.Contains(text, "created") || strings.Contains(text, "wall") {
		t.Fatalf("manifest contains a wall-clock field: %s", text)
	}
}

func TestPlanSelectionRejectsInvalidContracts(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		request SelectionRequest
		code    ErrorCode
	}{
		{
			name:    "missing session start",
			input:   intervalJSON("missing", 0, logTime0, nil),
			request: selectionRequest("missing", logTime0, logTime0),
			code:    ErrorMissingSessionStart,
		},
		{
			name: "duplicate session start",
			input: pattyLog(
				`{"schema_version":4,"event_type":"session_start","session_id":"duplicate","log_time":"1970-01-01T00:00:00Z"}`,
				intervalJSON("duplicate", 0, logTime0, nil),
			),
			request: selectionRequest("duplicate", logTime0, logTime0),
			code:    ErrorDuplicateSessionStart,
		},
		{
			name:    "missing endpoint",
			input:   pattyLog(intervalJSON("endpoint", 0, logTime0, nil)),
			request: selectionRequest("endpoint", logTime0, logTime1),
			code:    ErrorMissingEndpoint,
		},
		{
			name: "ambiguous endpoint",
			input: pattyLog(
				intervalJSON("ambiguous", 0, logTime0, nil),
				intervalJSON("ambiguous", 1, "2026-08-08T15:01:00Z", nil),
			),
			request: selectionRequest("ambiguous", logTime0, logTime0),
			code:    ErrorAmbiguousEndpoint,
		},
		{
			name: "duplicate interval ID",
			input: pattyLog(
				intervalJSON("duplicate-id", 0, logTime0, nil),
				intervalJSON("duplicate-id", 0, logTime1, nil),
			),
			request: selectionRequest("duplicate-id", logTime0, logTime1),
			code:    ErrorDuplicateInterval,
		},
		{
			name: "file-order reversal",
			input: pattyLog(
				intervalJSON("reverse", 0, logTime2, nil),
				intervalJSON("reverse", 1, logTime1, nil),
			),
			request: selectionRequest("reverse", logTime1, logTime2),
			code:    ErrorReversedRange,
		},
		{
			name: "session boundary",
			input: strings.Join([]string{
				`{"schema_version":4,"event_type":"session_start","session_id":"boundary","log_time":"1970-01-01T00:00:00Z"}`,
				intervalJSON("boundary", 0, logTime0, nil),
				`{"schema_version":4,"event_type":"session_start","session_id":"other","log_time":"1970-01-01T00:00:00Z"}`,
				intervalJSON("boundary", 1, logTime1, nil),
			}, "\n"),
			request: selectionRequest("boundary", logTime0, logTime1),
			code:    ErrorSessionBoundary,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := PlanSelection(strings.NewReader(test.input), test.request)
			assertSelectionError(t, err, test.code)
		})
	}
}

func TestPlanSelectionRejectsInvalidRequestBeforeReading(t *testing.T) {
	request := selectionRequest("request", logTime1, logTime0)
	_, err := PlanSelection(strings.NewReader(""), request)
	assertSelectionError(t, err, ErrorReversedRange)
}

func TestWritePattyLogDetectsChangedInput(t *testing.T) {
	input := pattyLog(intervalJSON("changed", 0, logTime0, nil))
	plan, err := PlanSelection(strings.NewReader(input), selectionRequest("changed", logTime0, logTime0))
	if err != nil {
		t.Fatalf("plan selection: %v", err)
	}
	var output bytes.Buffer
	if err := plan.WritePattyLog(strings.NewReader(strings.Split(input, "\n")[0]), &output); err == nil || !strings.Contains(err.Error(), "changed after planning") {
		t.Fatalf("changed input error = %v", err)
	}
}

func selectionRequest(sessionID, from, through string) SelectionRequest {
	return SelectionRequest{
		SessionID:      sessionID,
		FromLogTime:    mustTime(from),
		ThroughLogTime: mustTime(through),
		SourceName:     "/tmp/source.jsonl",
		CreatorVersion: "0.1.8",
	}
}

func pattyLog(records ...string) string {
	lines := []string{`{"schema_version":4,"event_type":"session_start","session_id":"` + sessionID(records) + `","log_time":"1970-01-01T00:00:00Z"}`}
	lines = append(lines, records...)
	return strings.Join(lines, "\n")
}

func sessionID(records []string) string {
	if len(records) == 0 {
		return "session"
	}
	var record struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal([]byte(records[0]), &record); err != nil || record.SessionID == "" {
		return "session"
	}
	return record.SessionID
}

func intervalJSON(sessionID string, interval int, logTime string, sourceLines []string) string {
	record := map[string]any{
		"schema_version": 4,
		"event_type":     "interval",
		"session_id":     sessionID,
		"log_time":       logTime,
		"interval":       interval,
	}
	if sourceLines != nil {
		record["source_lines"] = sourceLines
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func mustTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		panic(err)
	}
	return parsed
}

func assertSelectionError(t *testing.T, err error, code ErrorCode) {
	t.Helper()
	var selectionErr *SelectionError
	if !errors.As(err, &selectionErr) {
		t.Fatalf("error = %v, want SelectionError %q", err, code)
	}
	if selectionErr.Code != code {
		t.Fatalf("error code = %q, want %q (%v)", selectionErr.Code, code, err)
	}
}

func equalInts(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
