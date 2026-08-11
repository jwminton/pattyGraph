// Package investigation defines the durable PattyLog selection contract used
// by incident bundles. Archive creation and presentation are separate layers.
package investigation

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	BundleSchemaVersion = 1
	BundleType          = "pattygraph_incident"
	PattyLogEntryName   = "pattyLog.jsonl"
)

type CreatorManifest struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type PattyLogManifest struct {
	Entry                   string `json:"entry"`
	Representation          string `json:"representation"`
	SourceName              string `json:"source_name"`
	SessionID               string `json:"session_id"`
	SchemaVersions          []int  `json:"schema_versions"`
	RecordCount             int    `json:"record_count"`
	RetainedSourceIntervals int    `json:"retained_source_intervals"`
}

type RangeManifest struct {
	FromLogTime     string `json:"from_log_time"`
	ThroughLogTime  string `json:"through_log_time"`
	FromInterval    int    `json:"from_interval"`
	ThroughInterval int    `json:"through_interval"`
	IntervalCount   int    `json:"interval_count"`
}

type Manifest struct {
	BundleSchema int              `json:"bundle_schema"`
	BundleType   string           `json:"bundle_type"`
	Creator      CreatorManifest  `json:"creator"`
	PattyLog     PattyLogManifest `json:"pattylog"`
	Range        RangeManifest    `json:"range"`
}

type SelectionRequest struct {
	SessionID      string
	FromLogTime    time.Time
	ThroughLogTime time.Time
	SourceName     string
	CreatorVersion string
}

type ErrorCode string

const (
	ErrorInvalidRequest        ErrorCode = "invalid_request"
	ErrorInvalidRecord         ErrorCode = "invalid_record"
	ErrorAmbiguousSession      ErrorCode = "ambiguous_session"
	ErrorMissingSessionStart   ErrorCode = "missing_session_start"
	ErrorDuplicateSessionStart ErrorCode = "duplicate_session_start"
	ErrorMissingEndpoint       ErrorCode = "missing_endpoint"
	ErrorAmbiguousEndpoint     ErrorCode = "ambiguous_endpoint"
	ErrorReversedRange         ErrorCode = "reversed_range"
	ErrorDuplicateInterval     ErrorCode = "duplicate_interval"
	ErrorSessionBoundary       ErrorCode = "session_boundary"
)

type SelectionError struct {
	Code   ErrorCode
	Line   int
	Detail string
}

func (e *SelectionError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("%s at PattyLog line %d: %s", e.Code, e.Line, e.Detail)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Detail)
}

// SelectionPlan contains stable line membership and manifest metadata. It does
// not retain PattyLog payloads, so large interval records remain streamable.
type SelectionPlan struct {
	Manifest      Manifest
	includedLines map[int]struct{}
}

type recordEnvelope struct {
	SchemaVersion *int              `json:"schema_version"`
	EventType     string            `json:"event_type"`
	SessionID     string            `json:"session_id"`
	LogTime       string            `json:"log_time"`
	Timestamp     string            `json:"timestamp"`
	Interval      *int              `json:"interval"`
	SourceLines   []json.RawMessage `json:"source_lines"`
}

type pendingRecord struct {
	line          int
	malformed     bool
	eventType     string
	sessionID     string
	interval      *int
	schemaVersion *int
}

type planner struct {
	request     SelectionRequest
	autoSession bool

	includedLines     map[int]struct{}
	schemaVersions    map[int]struct{}
	selectedIntervals map[int]struct{}
	seenIntervals     map[int]int
	availableSessions map[string]struct{}
	pending           []pendingRecord

	targetSessionActive bool
	insideRange         bool
	rangeComplete       bool
	interrupted         bool

	sessionStartLine   int
	sessionStartCount  int
	sessionStartSchema *int
	fromMatches        []intervalEndpoint
	throughMatches     []intervalEndpoint

	fromEndpoint            intervalEndpoint
	throughEndpoint         intervalEndpoint
	intervalCount           int
	retainedSourceIntervals int
}

type intervalEndpoint struct {
	line     int
	interval int
	logTime  string
}

// PlanSelection validates one session and computes exact JSONL line membership
// for an inclusive pair of emitted interval log-times. An empty session ID is
// inferred only when the PattyLog contains one session.
func PlanSelection(input io.ReadSeeker, request SelectionRequest) (*SelectionPlan, error) {
	if err := validateRequest(request); err != nil {
		return nil, err
	}
	if _, err := input.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek PattyLog: %w", err)
	}

	p := &planner{
		request:           request,
		autoSession:       strings.TrimSpace(request.SessionID) == "",
		includedLines:     make(map[int]struct{}),
		schemaVersions:    make(map[int]struct{}),
		selectedIntervals: make(map[int]struct{}),
		seenIntervals:     make(map[int]int),
		availableSessions: make(map[string]struct{}),
	}
	if err := readJSONLLines(input, p.visitLine); err != nil {
		return nil, err
	}
	p.flushPending(false)
	if err := p.validateResult(); err != nil {
		return nil, err
	}

	p.includeLine(p.sessionStartLine, p.sessionStartSchema)
	schemaVersions := make([]int, 0, len(p.schemaVersions))
	for version := range p.schemaVersions {
		schemaVersions = append(schemaVersions, version)
	}
	sort.Ints(schemaVersions)

	return &SelectionPlan{
		Manifest: Manifest{
			BundleSchema: BundleSchemaVersion,
			BundleType:   BundleType,
			Creator:      CreatorManifest{Name: "pattyView", Version: request.CreatorVersion},
			PattyLog: PattyLogManifest{
				Entry:                   PattyLogEntryName,
				Representation:          "source",
				SourceName:              filepath.Base(request.SourceName),
				SessionID:               p.request.SessionID,
				SchemaVersions:          schemaVersions,
				RecordCount:             len(p.includedLines),
				RetainedSourceIntervals: p.retainedSourceIntervals,
			},
			Range: RangeManifest{
				FromLogTime:     p.fromEndpoint.logTime,
				ThroughLogTime:  p.throughEndpoint.logTime,
				FromInterval:    p.fromEndpoint.interval,
				ThroughInterval: p.throughEndpoint.interval,
				IntervalCount:   p.intervalCount,
			},
		},
		includedLines: p.includedLines,
	}, nil
}

// WritePattyLog copies selected source lines in their original file order and
// normalizes record terminators to one LF. It never rewrites JSON payloads.
func (p *SelectionPlan) WritePattyLog(input io.ReadSeeker, output io.Writer) error {
	if p == nil {
		return errors.New("nil investigation selection plan")
	}
	if _, err := input.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek PattyLog: %w", err)
	}
	written := 0
	err := readJSONLLines(input, func(lineNumber int, line string) error {
		if _, ok := p.includedLines[lineNumber]; !ok {
			return nil
		}
		if _, err := io.WriteString(output, line+"\n"); err != nil {
			return fmt.Errorf("write selected PattyLog line %d: %w", lineNumber, err)
		}
		written++
		return nil
	})
	if err != nil {
		return err
	}
	if written != len(p.includedLines) {
		return fmt.Errorf("selected PattyLog changed after planning: wrote %d of %d records", written, len(p.includedLines))
	}
	return nil
}

func validateRequest(request SelectionRequest) error {
	switch {
	case request.FromLogTime.IsZero():
		return &SelectionError{Code: ErrorInvalidRequest, Detail: "from log-time is required"}
	case request.ThroughLogTime.IsZero():
		return &SelectionError{Code: ErrorInvalidRequest, Detail: "through log-time is required"}
	case request.FromLogTime.After(request.ThroughLogTime):
		return &SelectionError{Code: ErrorReversedRange, Detail: "from log-time is after through log-time"}
	case strings.TrimSpace(request.SourceName) == "":
		return &SelectionError{Code: ErrorInvalidRequest, Detail: "source PattyLog name is required"}
	case strings.TrimSpace(request.CreatorVersion) == "":
		return &SelectionError{Code: ErrorInvalidRequest, Detail: "creator version is required"}
	default:
		return nil
	}
}

func (p *planner) visitLine(lineNumber int, line string) error {
	if strings.TrimSpace(line) == "" {
		return nil
	}
	var record recordEnvelope
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		if p.targetSessionActive {
			p.pending = append(p.pending, pendingRecord{line: lineNumber, malformed: true})
		}
		return nil
	}

	if record.EventType == "session_start" {
		p.flushPending(false)
		if strings.TrimSpace(record.SessionID) == "" {
			return &SelectionError{Code: ErrorInvalidRecord, Line: lineNumber, Detail: "session_start has no session ID"}
		}
		p.availableSessions[record.SessionID] = struct{}{}
		if p.autoSession && p.request.SessionID == "" {
			p.request.SessionID = record.SessionID
		}
		if record.SessionID == p.request.SessionID {
			p.sessionStartCount++
			p.sessionStartLine = lineNumber
			p.sessionStartSchema = record.SchemaVersion
			p.targetSessionActive = true
			if p.insideRange && !p.rangeComplete {
				p.interrupted = true
			}
		} else {
			if p.insideRange && !p.rangeComplete {
				p.interrupted = true
			}
			p.targetSessionActive = false
		}
		return nil
	}

	if record.SessionID != p.request.SessionID || !p.targetSessionActive {
		return nil
	}
	meta := pendingRecord{
		line:          lineNumber,
		eventType:     record.EventType,
		sessionID:     record.SessionID,
		interval:      record.Interval,
		schemaVersion: record.SchemaVersion,
	}
	if record.EventType != "interval" {
		p.pending = append(p.pending, meta)
		return nil
	}
	if record.Interval == nil {
		return &SelectionError{Code: ErrorInvalidRecord, Line: lineNumber, Detail: "interval record has no interval ID"}
	}
	if previousLine, exists := p.seenIntervals[*record.Interval]; exists {
		return &SelectionError{
			Code:   ErrorDuplicateInterval,
			Line:   lineNumber,
			Detail: fmt.Sprintf("interval %d was already recorded at line %d", *record.Interval, previousLine),
		}
	}
	p.seenIntervals[*record.Interval] = lineNumber

	logTimeText := record.LogTime
	if logTimeText == "" {
		logTimeText = record.Timestamp
	}
	logTime, err := time.Parse(time.RFC3339Nano, logTimeText)
	if err != nil {
		return &SelectionError{Code: ErrorInvalidRecord, Line: lineNumber, Detail: "interval has no valid log_time or timestamp"}
	}
	endpoint := intervalEndpoint{line: lineNumber, interval: *record.Interval, logTime: logTimeText}
	if logTime.Equal(p.request.FromLogTime) {
		p.fromMatches = append(p.fromMatches, endpoint)
	}
	if logTime.Equal(p.request.ThroughLogTime) {
		p.throughMatches = append(p.throughMatches, endpoint)
	}

	selected := p.insideRange && !p.rangeComplete
	if logTime.Equal(p.request.FromLogTime) {
		p.insideRange = true
		selected = true
		p.fromEndpoint = endpoint
	}
	if selected {
		p.selectedIntervals[*record.Interval] = struct{}{}
	}
	p.flushPending(selected)
	if selected {
		p.includeLine(lineNumber, record.SchemaVersion)
		p.intervalCount++
		if len(record.SourceLines) > 0 {
			p.retainedSourceIntervals++
		}
	}
	if selected && logTime.Equal(p.request.ThroughLogTime) {
		p.rangeComplete = true
		p.throughEndpoint = endpoint
	}
	return nil
}

func (p *planner) flushPending(nextIntervalSelected bool) {
	for _, record := range p.pending {
		include := nextIntervalSelected
		if record.eventType == "alert" {
			include = false
			if record.interval != nil {
				_, include = p.selectedIntervals[*record.interval]
			}
		}
		if include {
			p.includeLine(record.line, record.schemaVersion)
		}
	}
	p.pending = p.pending[:0]
}

func (p *planner) includeLine(line int, schemaVersion *int) {
	if line <= 0 {
		return
	}
	p.includedLines[line] = struct{}{}
	if schemaVersion != nil {
		p.schemaVersions[*schemaVersion] = struct{}{}
	}
}

func (p *planner) validateResult() error {
	switch {
	case p.autoSession && len(p.availableSessions) > 1:
		sessions := make([]string, 0, len(p.availableSessions))
		for sessionID := range p.availableSessions {
			sessions = append(sessions, sessionID)
		}
		sort.Strings(sessions)
		return &SelectionError{Code: ErrorAmbiguousSession, Detail: fmt.Sprintf("PattyLog contains multiple sessions; select one of %s", strings.Join(sessions, ", "))}
	case p.sessionStartCount == 0:
		if p.request.SessionID == "" {
			return &SelectionError{Code: ErrorMissingSessionStart, Detail: "PattyLog has no session_start record"}
		}
		return &SelectionError{Code: ErrorMissingSessionStart, Detail: fmt.Sprintf("session %q has no session_start record", p.request.SessionID)}
	case p.sessionStartCount > 1:
		return &SelectionError{Code: ErrorDuplicateSessionStart, Line: p.sessionStartLine, Detail: fmt.Sprintf("session %q has multiple session_start records", p.request.SessionID)}
	case p.interrupted:
		return &SelectionError{Code: ErrorSessionBoundary, Detail: "selected interval range crosses a session boundary"}
	case len(p.fromMatches) == 0:
		return &SelectionError{Code: ErrorMissingEndpoint, Detail: "from log-time does not identify an interval in the selected session"}
	case len(p.fromMatches) > 1:
		return &SelectionError{Code: ErrorAmbiguousEndpoint, Detail: "from log-time identifies multiple intervals in the selected session"}
	case len(p.throughMatches) == 0:
		return &SelectionError{Code: ErrorMissingEndpoint, Detail: "through log-time does not identify an interval in the selected session"}
	case len(p.throughMatches) > 1:
		return &SelectionError{Code: ErrorAmbiguousEndpoint, Detail: "through log-time identifies multiple intervals in the selected session"}
	case p.fromMatches[0].line > p.throughMatches[0].line:
		return &SelectionError{Code: ErrorReversedRange, Detail: "from interval follows through interval in PattyLog order"}
	case !p.rangeComplete || p.intervalCount == 0:
		return &SelectionError{Code: ErrorMissingEndpoint, Detail: "selected interval range was not completed"}
	case p.sessionStartLine > p.fromMatches[0].line:
		return &SelectionError{Code: ErrorInvalidRecord, Line: p.sessionStartLine, Detail: "session_start follows the selected interval range"}
	default:
		return nil
	}
}

func readJSONLLines(input io.Reader, visit func(lineNumber int, line string) error) error {
	reader := bufio.NewReader(input)
	lineNumber := 0
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			lineNumber++
			line = strings.TrimSuffix(line, "\n")
			line = strings.TrimSuffix(line, "\r")
			if visitErr := visit(lineNumber, line); visitErr != nil {
				return visitErr
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read PattyLog line %d: %w", lineNumber+1, err)
		}
	}
}
