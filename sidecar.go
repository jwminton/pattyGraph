// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	SidecarPhaseBeforePush = "before_push"
	SidecarPhaseAfterPush  = "after_push"

	SidecarEventSessionStart   = "session_start"
	SidecarEventInterval       = "interval"
	SidecarEventControlCommand = "control_command"
	SidecarEventAlert          = "alert"
	SidecarSchemaVersion       = 4

	SidecarMarkedStateMarked   = "marked"
	SidecarMarkedStateUnmarked = "unmarked"

	defaultSidecarPrefix   = "pattyLog_"
	defaultSidecarSuffix   = ".jsonl"
	defaultSidecarTopLimit = 25
)

var sidecarSessionID = newSidecarSessionID()

// SidecarOptions controls how much state is copied into each interval snapshot.
// The caller should take PattyGraph's mutex before calling snapshot methods if
// ingestion or push can run concurrently.
type SidecarOptions struct {
	Phase              string
	TopLimit           int
	FactoidLimit       int
	IncludeHistories   bool
	IncludeSourceLines bool
	IncludeMatcherKeys bool
}

func DefaultSidecarOptions() SidecarOptions {
	return SidecarOptions{
		Phase:              SidecarPhaseBeforePush,
		TopLimit:           defaultSidecarTopLimit,
		FactoidLimit:       8,
		IncludeHistories:   false,
		IncludeSourceLines: false,
		IncludeMatcherKeys: true,
	}
}

type SidecarInterval struct {
	SchemaVersion int                    `json:"schema_version"`
	EventType     string                 `json:"event_type"`
	SessionID     string                 `json:"session_id"`
	Timestamp     time.Time              `json:"timestamp"`
	Phase         string                 `json:"phase"`
	FilePath      string                 `json:"file_path"`
	Machine       string                 `json:"machine,omitempty"`
	CurrentCycle  int                    `json:"current_cycle"`
	LogicalCycles int                    `json:"logical_cycles"`
	Interval      int                    `json:"interval"`
	IntervalLines int                    `json:"interval_lines"`
	TotalLines    uint64                 `json:"total_lines"`
	TotalBytes    uint64                 `json:"total_bytes"`
	Unmarked      int                    `json:"unmarked"`
	LogTime       time.Time              `json:"log_time,omitempty"`
	Summary       SidecarSummary         `json:"summary"`
	Runtime       SidecarRuntime         `json:"runtime"`
	Matchers      []SidecarMatcher       `json:"matchers"`
	Interesting   []SidecarInteresting   `json:"interesting"`
	Factoids      []SidecarFactoid       `json:"factoids,omitempty"`
	Selected      SidecarSelectedContext `json:"selected,omitempty"`
}

type SidecarSessionStart struct {
	SchemaVersion         int       `json:"schema_version"`
	EventType             string    `json:"event_type"`
	SessionID             string    `json:"session_id"`
	Timestamp             time.Time `json:"timestamp"`
	FilePath              string    `json:"file_path"`
	Machine               string    `json:"machine,omitempty"`
	ProcessID             int       `json:"process_id"`
	Version               string    `json:"version,omitempty"`
	SaveDir               string    `json:"save_dir,omitempty"`
	Args                  []string  `json:"args,omitempty"`
	OutputPath            string    `json:"output_path,omitempty"`
	InlineCommandsEnabled bool      `json:"inline_commands_enabled"`
	ControlFileEnabled    bool      `json:"control_file_enabled"`
	ControlFilePath       string    `json:"control_file_path,omitempty"`
}

type SidecarControlCommand struct {
	SchemaVersion      int                    `json:"schema_version"`
	EventType          string                 `json:"event_type"`
	SessionID          string                 `json:"session_id"`
	Timestamp          time.Time              `json:"timestamp"`
	Source             string                 `json:"source"`
	Command            string                 `json:"command"`
	CommandName        string                 `json:"command_name,omitempty"`
	Status             string                 `json:"status"`
	ControlFileEnabled bool                   `json:"control_file_enabled"`
	ControlFilePath    string                 `json:"control_file_path,omitempty"`
	Result             map[string]interface{} `json:"result,omitempty"`
}

type SidecarAlert struct {
	SchemaVersion int       `json:"schema_version"`
	EventType     string    `json:"event_type"`
	SessionID     string    `json:"session_id"`
	Timestamp     time.Time `json:"timestamp"`
	Status        string    `json:"status"`
	Matcher       string    `json:"matcher"`
	Direction     string    `json:"direction"`
	Value         int       `json:"value"`
	Threshold     int       `json:"threshold"`
	FluxDepth     int       `json:"flux_depth"`
	Streak        int       `json:"streak"`
	Interval      int       `json:"interval"`
	CurrentCycle  int       `json:"current_cycle"`
}

type SidecarSummary struct {
	OverallMin       int     `json:"overall_min"`
	OverallAvgMax    float64 `json:"overall_avg_max"`
	OverallMax       int     `json:"overall_max"`
	LastMonitorMax   int     `json:"last_monitor_max"`
	LastLinesMax     int     `json:"last_lines_max"`
	LastBytesMax     int     `json:"last_bytes_max"`
	PeakLines        int     `json:"peak_lines"`
	PeakBytes        int     `json:"peak_bytes"`
	PeakErrs         int     `json:"peak_errs"`
	AvgLines         int     `json:"avg_lines"`
	AvgBytes         int     `json:"avg_bytes"`
	AvgErrs          int     `json:"avg_errs"`
	AutobotsMigrated int     `json:"autobots_migrated"`
	BotsIndex        int     `json:"bots_index"`
}

type SidecarRuntime struct {
	UptimeSeconds         int64              `json:"uptime_seconds"`
	KnownFactoids         int                `json:"known_factoids"`
	TotalAgentTokens      uint64             `json:"total_agent_tokens"`
	AgentTokenCardinality map[int]uint64     `json:"agent_token_cardinality,omitempty"`
	DisplayFreeze         int                `json:"display_freeze"`
	ExpertMode            bool               `json:"expert_mode"`
	ShowTicker            bool               `json:"show_ticker"`
	CurrentLine           *SidecarLineSource `json:"current_line,omitempty"`
}

type SidecarMatcher struct {
	Name              string              `json:"name"`
	Color             string              `json:"color,omitempty"`
	ColorHex          string              `json:"color_hex,omitempty"`
	IsHistorical      bool                `json:"is_historical"`
	IsAddedAutobot    bool                `json:"is_added_autobot"`
	UseRegexMatchKeys bool                `json:"use_regex_match_keys"`
	IntervalCount     int                 `json:"interval_count"`
	LastIntervalCount int                 `json:"last_interval_count"`
	History           []int               `json:"history,omitempty"`
	HistoryTotal      int                 `json:"history_total"`
	HistoryPeak       int                 `json:"history_peak"`
	TopKeys           []SidecarCountEntry `json:"top_keys,omitempty"`
	TopGroups         []SidecarGroupEntry `json:"top_groups,omitempty"`
	FirstLine         string              `json:"first_line,omitempty"`
	IntervalLine      string              `json:"interval_line,omitempty"`
	LastLine          string              `json:"last_line,omitempty"`
}

type SidecarCountEntry struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
	Rank  int    `json:"rank"`
}

type SidecarGroupEntry struct {
	Prefix  string `json:"prefix"`
	Count   int    `json:"count"`
	Members int    `json:"members"`
	Rank    int    `json:"rank"`
}

type SidecarInteresting struct {
	Name      string                 `json:"name"`
	Top       []SidecarWordEntry     `json:"top"`
	Peaks     []SidecarWordEntry     `json:"peaks,omitempty"`
	IPGroups  []SidecarIPGroupEntry  `json:"ip_groups,omitempty"`
	TotalKeys int                    `json:"total_keys"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

type SidecarWordEntry struct {
	Key               string             `json:"key"`
	Rank              int                `json:"rank"`
	Score             int                `json:"score"`
	Count             int                `json:"count"`
	Bytes             uint64             `json:"bytes,omitempty"`
	PrimeFlux         int                `json:"prime_flux"`
	Burstiness        float64            `json:"burstiness"`
	AgentDeltaMetric  float64            `json:"agent_delta_metric"`
	History           []int              `json:"history,omitempty"`
	HistoryTotal      int                `json:"history_total"`
	HistoryPeak       int                `json:"history_peak"`
	HistoryDepth      int                `json:"history_depth"`
	LastSeenTic       int                `json:"last_seen_tic"`
	LastStatus        string             `json:"last_status,omitempty"`
	Color             string             `json:"color,omitempty"`
	MarkedState       string             `json:"marked_state"`
	MarkedByMatcher   string             `json:"marked_by_matcher,omitempty"`
	IsPeak            bool               `json:"is_peak"`
	Source            *SidecarLineSource `json:"source,omitempty"`
	FirstIntervalLine string             `json:"first_interval_line,omitempty"`
	LastLine          string             `json:"last_line,omitempty"`
}

type SidecarIPGroupEntry struct {
	Prefix            string  `json:"prefix"`
	Rank              int     `json:"rank"`
	Score             int     `json:"score"`
	Count             int     `json:"count"`
	CountPlusFirst    int     `json:"count_plus_first"`
	Members           int     `json:"members"`
	Bytes             uint64  `json:"bytes,omitempty"`    // current-interval member sum
	Burstiness        float64 `json:"burstiness"`         // active-member average
	AgentDeltaMetric  float64 `json:"agent_delta_metric"` // active-member average
	History           []int   `json:"history,omitempty"`
	HistoryDepth      int     `json:"history_depth"`
	Color             string  `json:"color,omitempty"`
	MarkedState       string  `json:"marked_state"`
	MarkedByMatcher   string  `json:"marked_by_matcher,omitempty"`
	FirstLine         string  `json:"first_line,omitempty"`
	FirstIntervalLine string  `json:"first_interval_line,omitempty"`
	LastLine          string  `json:"last_line,omitempty"`
}

type SidecarFactoid struct {
	Name        string `json:"name,omitempty"`
	Text        string `json:"text"`
	Probability int    `json:"probability"`
}

type SidecarSelectedContext struct {
	GraphValue          int                `json:"graph_value,omitempty"`
	SelectionValue      string             `json:"selection_value,omitempty"`
	InterestingMatcher  string             `json:"interesting_matcher,omitempty"`
	InterestingKey      string             `json:"interesting_key,omitempty"`
	FirstSource         *SidecarLineSource `json:"first_source,omitempty"`
	FirstIntervalSource *SidecarLineSource `json:"first_interval_source,omitempty"`
	LastSource          *SidecarLineSource `json:"last_source,omitempty"`
	Matcher             string             `json:"matcher,omitempty"`
}

type SidecarLineSource struct {
	LogLine         string   `json:"log_line,omitempty"`
	IP              string   `json:"ip,omitempty"`
	IPPrefix        string   `json:"ip_prefix,omitempty"`
	Request         string   `json:"request,omitempty"`
	Referer         string   `json:"referer,omitempty"`
	BytesValue      int      `json:"bytes_value,omitempty"`
	UserAgent       string   `json:"user_agent,omitempty"`
	ResponseCode    string   `json:"response_code,omitempty"`
	UserAgentDelta  float64  `json:"user_agent_delta,omitempty"`
	CaptureColor    string   `json:"capture_color,omitempty"`
	MarkedState     string   `json:"marked_state"`
	MarkedByMatcher string   `json:"marked_by_matcher,omitempty"`
	TokenBandCount  int      `json:"token_band_count,omitempty"`
	UserAgentTokens []string `json:"user_agent_tokens,omitempty"`
}

func (m *Monitor) SidecarSnapshotBeforePush() SidecarInterval {
	opts := DefaultSidecarOptions()
	opts.Phase = SidecarPhaseBeforePush
	return m.SidecarSnapshot(opts)
}

func (m *Monitor) SidecarSnapshotAfterPush() SidecarInterval {
	opts := DefaultSidecarOptions()
	opts.Phase = SidecarPhaseAfterPush
	return m.SidecarSnapshot(opts)
}

func (m *Monitor) SidecarSnapshot(opts SidecarOptions) SidecarInterval {
	opts = normalizeSidecarOptions(opts)
	minHistory, avgMax, maxHistory := m.minAvgMaxHistoryAcrossMatchers()

	return SidecarInterval{
		SchemaVersion: SidecarSchemaVersion,
		EventType:     SidecarEventInterval,
		SessionID:     sidecarSessionID,
		Timestamp:     sidecarEventTime(m),
		Phase:         opts.Phase,
		FilePath:      m.filePath,
		Machine:       machineDisplayName,
		CurrentCycle:  currentCycle,
		LogicalCycles: logicalCycles,
		Interval:      m.intervalsCompleted,
		IntervalLines: m.intervalLines,
		TotalLines:    m.totalLines,
		TotalBytes:    m.totalBytes,
		Unmarked:      m.unmarked,
		LogTime:       m.logtime,
		Summary: SidecarSummary{
			OverallMin:       minHistory,
			OverallAvgMax:    avgMax,
			OverallMax:       maxHistory,
			LastMonitorMax:   lastMonitorMaxBuf.Latest(),
			LastLinesMax:     lastLinesBuf.Latest(),
			LastBytesMax:     lastBytesBuf.Latest(),
			PeakLines:        maxIntSlice(PattyGraph.linesMatcher.history),
			PeakBytes:        maxIntSlice(PattyGraph.bytesMatcher.history),
			PeakErrs:         maxIntSlice(PattyGraph.errsMatcher.history),
			AvgLines:         averageIntSlice(PattyGraph.linesMatcher.history),
			AvgBytes:         averageIntSlice(PattyGraph.bytesMatcher.history),
			AvgErrs:          avgErrs(),
			AutobotsMigrated: botsMigrated,
			BotsIndex:        botsIndex,
		},
		Runtime: SidecarRuntime{
			UptimeSeconds:         sidecarUptimeSeconds(),
			KnownFactoids:         len(facts.facts),
			TotalAgentTokens:      totalAgentTokenCount,
			AgentTokenCardinality: copyUint64Map(uaCardinalityMap),
			DisplayFreeze:         displayFreezeCountdown,
			ExpertMode:            expertMode,
			ShowTicker:            m.showTicker,
			CurrentLine:           sidecarLineSource(currentLine, opts.IncludeSourceLines),
		},
		Matchers:    sidecarMatcherSnapshots(m, opts),
		Interesting: sidecarInterestingSnapshots(m, opts),
		Factoids:    sidecarFactoids(opts.FactoidLimit),
		Selected:    sidecarSelectedContext(m),
	}
}

func (m *Monitor) SidecarSessionStart() SidecarSessionStart {
	event := SidecarSessionStart{
		SchemaVersion:         SidecarSchemaVersion,
		EventType:             SidecarEventSessionStart,
		SessionID:             sidecarSessionID,
		Timestamp:             time.Now(),
		FilePath:              m.filePath,
		Machine:               machineDisplayName,
		ProcessID:             os.Getpid(),
		Version:               PattyGraphVersion,
		Args:                  copyStringSlice(os.Args),
		InlineCommandsEnabled: true,
		ControlFileEnabled:    enableControlFile,
		ControlFilePath:       controlFilePath(),
	}
	if m != nil && m.pattyConfig != nil {
		event.SaveDir = m.pattyConfig.saveDir
	}
	event.OutputPath = m.SidecarDefaultPath()
	return event
}

func (m *Monitor) SidecarDefaultPath() string {
	if m != nil && m.pattyConfig != nil && m.pattyConfig.jsonFile != "" {
		return m.pattyConfig.jsonFile
	}
	if m != nil && m.pattyConfig != nil && m.pattyConfig.saveDir != "" {
		return filepath.Join(m.pattyConfig.saveDir, sidecarSessionFilename())
	}
	return sidecarSessionFilename()
}

func (m *Monitor) WriteSidecarSessionStartJSONL(path string) error {
	if path == "" && m != nil && m.pattyConfig != nil && m.pattyConfig.jsonFile != "" {
		if err := truncateSidecarJSONL(m.pattyConfig.jsonFile); err != nil {
			return err
		}
	}
	return m.writeSidecarEventJSONL(m.SidecarSessionStart(), path)
}

func truncateSidecarJSONL(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	return f.Close()
}

func (m *Monitor) SidecarControlCommand(command string, source string, result InlineCommandResult) SidecarControlCommand {
	return SidecarControlCommand{
		SchemaVersion:      SidecarSchemaVersion,
		EventType:          SidecarEventControlCommand,
		SessionID:          sidecarSessionID,
		Timestamp:          time.Now(),
		Source:             source,
		Command:            command,
		CommandName:        result.CommandName,
		Status:             result.Status,
		ControlFileEnabled: enableControlFile,
		ControlFilePath:    controlFilePath(),
		Result:             result.Result,
	}
}

func (m *Monitor) WriteSidecarControlCommandJSONL(command string, source string, result InlineCommandResult, path string) error {
	return m.writeSidecarEventJSONL(m.SidecarControlCommand(command, source, result), path)
}

func (m *Monitor) SidecarAlert(transition AlertTransition) SidecarAlert {
	return SidecarAlert{
		SchemaVersion: SidecarSchemaVersion,
		EventType:     SidecarEventAlert,
		SessionID:     sidecarSessionID,
		Timestamp:     sidecarEventTime(m),
		Status:        transition.Status,
		Matcher:       transition.MatcherName,
		Direction:     transition.Direction,
		Value:         transition.Value,
		Threshold:     transition.Threshold,
		FluxDepth:     transition.FluxDepth,
		Streak:        transition.Streak,
		Interval:      transition.Interval,
		CurrentCycle:  transition.CurrentCycle,
	}
}

func (m *Monitor) WriteSidecarAlertJSONL(transition AlertTransition, path string) error {
	return m.writeSidecarEventJSONL(m.SidecarAlert(transition), path)
}

func (m *Monitor) WriteSidecarJSONL(snapshot SidecarInterval, path string) error {
	return m.writeSidecarEventJSONL(snapshot, path)
}

const sidecarWriteFailureLimit = 3

var sidecarWriteFailures int

func recordSidecarWriteResult(label string, err error) {
	if err == nil {
		sidecarWriteFailures = 0
		return
	}
	sidecarWriteFailures++
	if sidecarWriteFailures >= sidecarWriteFailureLimit {
		msg := fmt.Sprintf("PattyLog %s failed %d times; JSONL disabled", label, sidecarWriteFailures)
		pushErrorNow("%s", msg)
		log.Printf("%s: %v", msg, err)
		generateSidecarJSONL = false
		return
	}
	pushErrorNow("PattyLog %s write failed %d/%d", label, sidecarWriteFailures, sidecarWriteFailureLimit)
	log.Printf("PattyLog %s write failed: %v", label, err)
}

func (m *Monitor) writeSidecarEventJSONL(event interface{}, path string) error {
	if path == "" {
		path = m.SidecarDefaultPath()
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	return enc.Encode(event)
}

func (m *Monitor) WriteSidecarSnapshotJSONL(opts SidecarOptions, path string) error {
	return m.WriteSidecarJSONL(m.SidecarSnapshot(opts), path)
}

func normalizeSidecarOptions(opts SidecarOptions) SidecarOptions {
	if opts.Phase == "" {
		opts.Phase = SidecarPhaseBeforePush
	}
	if opts.TopLimit <= 0 {
		opts.TopLimit = defaultSidecarTopLimit
	}
	return opts
}

func sidecarMatcherSnapshots(m *Monitor, opts SidecarOptions) []SidecarMatcher {
	out := make([]SidecarMatcher, 0, len(m.matchers))
	for _, mf := range m.matchers {
		matcher := mf.asMatcher()
		if matcher == nil {
			continue
		}
		out = append(out, sidecarMatcherSnapshot(matcher, opts))
	}
	return out
}

func sidecarMatcherSnapshot(m *Matcher, opts SidecarOptions) SidecarMatcher {
	history := copyIntSlice(m.history)
	s := SidecarMatcher{
		Name:              m.matcherName(),
		Color:             strings.Trim(m.color, "[]"),
		ColorHex:          tcellColorToHex(m.color),
		IsHistorical:      m.isHistorical,
		IsAddedAutobot:    m.isAddedAutobot,
		UseRegexMatchKeys: m.useRegexMatchKeys,
		IntervalCount:     m.intervalCount,
		LastIntervalCount: m.lastIntervalCount,
		HistoryTotal:      sumIntSlice(history),
		HistoryPeak:       maxIntSlice(history),
	}
	if opts.IncludeHistories {
		s.History = history
	}
	if opts.IncludeMatcherKeys {
		s.TopKeys = sidecarCountEntries(m.matchCountsMap, opts.TopLimit)
		s.TopGroups = sidecarGroupEntries(m.ipGroupsCountsMap, m.ipGroupsMap, opts.TopLimit)
	}
	if opts.IncludeSourceLines {
		s.FirstLine = m.firstMatchLine
		s.IntervalLine = m.intervalMatchLine
		s.LastLine = m.lastMatchLine
	}
	return s
}

func sidecarInterestingSnapshots(m *Monitor, opts SidecarOptions) []SidecarInteresting {
	return []SidecarInteresting{
		sidecarInterestingSnapshot(m.wordsMatcher, opts),
		sidecarInterestingSnapshot(m.refsMatcher, opts),
		sidecarInterestingSnapshot(m.ipsMatcher, opts),
	}
}

func sidecarInterestingSnapshot(m *InterestingWordMatcher, opts SidecarOptions) SidecarInteresting {
	if m == nil {
		return SidecarInteresting{}
	}
	out := SidecarInteresting{
		Name:      m.mName,
		Top:       sidecarWordEntries(m, opts.TopLimit, opts),
		Peaks:     sidecarPeakWordEntries(m, opts.TopLimit, opts),
		TotalKeys: len(m.wordFrequency),
		Metadata: map[string]interface{}{
			"push_interval_count": m.pushIntervalCount,
			"word_stats_created":  m.wordStatsCreated,
			"selected_key":        m.selectedKey,
			"display_width":       m.displayWidth,
		},
	}
	if m.mName == "ips" && m.ipScratch != nil {
		out.IPGroups = sidecarIPGroupEntries(m, opts.TopLimit, opts)
	}
	return out
}

func sidecarWordEntries(m *InterestingWordMatcher, limit int, opts SidecarOptions) []SidecarWordEntry {
	entries := make([]SidecarWordEntry, 0, len(m.wordFrequency))
	for key, stats := range m.wordFrequency {
		entries = append(entries, sidecarWordEntry(m, key, stats, opts))
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Score == entries[j].Score {
			return entries[i].Key < entries[j].Key
		}
		return entries[i].Score > entries[j].Score
	})
	return rankAndLimitWordEntries(entries, limit)
}

func sidecarPeakWordEntries(m *InterestingWordMatcher, limit int, opts SidecarOptions) []SidecarWordEntry {
	entries := make([]SidecarWordEntry, 0, len(m.peakWords))
	for _, key := range m.peakWords {
		stats := m.wordFrequency[key]
		if stats == nil {
			continue
		}
		entries = append(entries, sidecarWordEntry(m, key, stats, opts))
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Score == entries[j].Score {
			return entries[i].Key < entries[j].Key
		}
		return entries[i].Score > entries[j].Score
	})
	return rankAndLimitWordEntries(entries, limit)
}

func sidecarWordEntry(m *InterestingWordMatcher, key string, stats *WordStats, opts SidecarOptions) SidecarWordEntry {
	history := stats.historySlice()
	historyCopy := copyIntSlice(history)
	source := sidecarLineSource(stats.source, opts.IncludeSourceLines)
	score := stats.count + stats.primeFlux
	markedState, markedByMatcher := sidecarMarkedFields(stats.source)

	entry := SidecarWordEntry{
		Key:              key,
		Score:            score,
		Count:            stats.count,
		Bytes:            stats.bytes,
		PrimeFlux:        stats.primeFlux,
		Burstiness:       stats.burstiness(),
		AgentDeltaMetric: stats.agentDeltaMetric,
		HistoryTotal:     sumIntSlice(historyCopy),
		HistoryPeak:      maxIntSlice(historyCopy),
		HistoryDepth:     stats.historyLength(),
		LastSeenTic:      stats.lastSeenTic,
		LastStatus:       stats.lastStatus,
		Color:            stats.captureColor(),
		MarkedState:      markedState,
		MarkedByMatcher:  markedByMatcher,
		IsPeak:           m.peakWordsSet[key],
		Source:           source,
	}
	if opts.IncludeHistories {
		entry.History = historyCopy
	}
	if opts.IncludeSourceLines {
		entry.FirstIntervalLine = stats.firstIntervalLogLine
		entry.LastLine = stats.lastLogLine
	}
	return entry
}

type sidecarIPGroupMetrics struct {
	bytes             uint64
	burstiness        float64
	agentDeltaMetric  float64
	contributingCount int
}

// sidecarMetricsForIPGroup derives structured values independently of the
// active TUI tab. This interval-only pass deliberately avoids adding work to
// displayIpGroups(), PattyGraph's most performance-sensitive display path.
func sidecarMetricsForIPGroup(m *InterestingWordMatcher, prefix string) sidecarIPGroupMetrics {
	var metrics sidecarIPGroupMetrics
	for _, ip := range m.ipScratch.ActivePrefixMembers(prefix) {
		stats, exists := m.wordFrequency[ip]
		if !exists || stats == nil {
			continue
		}
		metrics.bytes += stats.bytes
		metrics.burstiness += stats.burstiness()
		metrics.agentDeltaMetric += stats.agentDeltaMetric
		metrics.contributingCount++
	}
	if metrics.contributingCount > 0 {
		members := float64(metrics.contributingCount)
		metrics.burstiness /= members
		metrics.agentDeltaMetric /= members
	}
	return metrics
}

func sidecarIPGroupEntries(m *InterestingWordMatcher, limit int, opts SidecarOptions) []SidecarIPGroupEntry {
	defer m.topTracker.Reset()
	_ = m.topWordEntries()
	_, _ = m.displayIpGroups()

	entries := make([]SidecarIPGroupEntry, 0, len(m.ipScratch.prefixCounts))
	for prefix, count := range m.ipScratch.prefixCounts {
		history := []int(nil)
		if acc := m.ipScratch.prefixHistorAggregateBufs[prefix]; acc != nil {
			history = acc.Slice()
		}
		score := count + m.ipScratch.prefixFirstPlusCounts[prefix]
		markedState, markedByMatcher := sidecarMarkedFields(&lineSource{
			captureColor:   m.ipScratch.prefixColors[prefix],
			captureMatcher: m.ipScratch.prefixMatchers[prefix],
		})
		entry := SidecarIPGroupEntry{
			Prefix:          prefix,
			Score:           score,
			Count:           count,
			CountPlusFirst:  m.ipScratch.prefixFirstPlusCounts[prefix],
			Members:         m.ipScratch.prefixMembers[prefix],
			HistoryDepth:    m.ipScratch.prefixDepths[prefix],
			Color:           strings.Trim(m.ipScratch.prefixColors[prefix], "[]"),
			MarkedState:     markedState,
			MarkedByMatcher: markedByMatcher,
		}
		if opts.IncludeHistories {
			entry.History = history
		}
		if opts.IncludeSourceLines {
			entry.FirstLine = m.ipScratch.prefixFirstLines[prefix]
			entry.FirstIntervalLine = m.ipScratch.prefixFirstIntervalLines[prefix]
			entry.LastLine = m.ipScratch.prefixLastLines[prefix]
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Score == entries[j].Score {
			return entries[i].Prefix < entries[j].Prefix
		}
		return entries[i].Score > entries[j].Score
	})
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}
	for i := range entries {
		metrics := sidecarMetricsForIPGroup(m, entries[i].Prefix)
		entries[i].Bytes = metrics.bytes
		entries[i].Burstiness = metrics.burstiness
		entries[i].AgentDeltaMetric = metrics.agentDeltaMetric
		entries[i].Rank = i + 1
	}
	return entries
}

func sidecarCountEntries(counts map[string]int, limit int) []SidecarCountEntry {
	entries := make([]SidecarCountEntry, 0, len(counts))
	for key, count := range counts {
		if count <= 0 {
			continue
		}
		entries = append(entries, SidecarCountEntry{Key: key, Count: count})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Count == entries[j].Count {
			return entries[i].Key < entries[j].Key
		}
		return entries[i].Count > entries[j].Count
	})
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}
	for i := range entries {
		entries[i].Rank = i + 1
	}
	return entries
}

func sidecarGroupEntries(counts, members map[string]int, limit int) []SidecarGroupEntry {
	entries := make([]SidecarGroupEntry, 0, len(counts))
	for prefix, count := range counts {
		if count <= 0 {
			continue
		}
		entries = append(entries, SidecarGroupEntry{
			Prefix:  prefix,
			Count:   count,
			Members: members[prefix],
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Count == entries[j].Count {
			return entries[i].Prefix < entries[j].Prefix
		}
		return entries[i].Count > entries[j].Count
	})
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}
	for i := range entries {
		entries[i].Rank = i + 1
	}
	return entries
}

func rankAndLimitWordEntries(entries []SidecarWordEntry, limit int) []SidecarWordEntry {
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}
	for i := range entries {
		entries[i].Rank = i + 1
	}
	return entries
}

func sidecarFactoids(limit int) []SidecarFactoid {
	if limit <= 0 {
		return nil
	}
	out := make([]SidecarFactoid, 0, limit)
	for len(out) < limit {
		text, probability, name := facts.NextBackground()
		text = strings.TrimSpace(stripBrackets(text))
		if text == "" {
			break
		}
		out = append(out, SidecarFactoid{
			Name:        name,
			Text:        text,
			Probability: probability,
		})
	}
	return out
}

func sidecarSelectedContext(m *Monitor) SidecarSelectedContext {
	out := SidecarSelectedContext{
		GraphValue:     m.selectedGraphValue,
		SelectionValue: m.selectionValue,
	}
	if m.selectedInterestingMatcher != nil {
		selected := m.selectedInterestingMatcher
		out.InterestingMatcher = selected.mName
		out.InterestingKey = selected.selectedKey
		if stats := selected.wordFrequency[selected.selectedKey]; stats != nil {
			sidecarFillSelectedWordStats(&out, stats)
		} else if selected.ipScratch != nil {
			if stats := selected.ipScratch.prefixStats[selected.selectedKey]; stats != nil {
				sidecarFillSelectedWordStats(&out, stats)
			}
		}
	}
	if m.selectedMatcher != nil {
		out.Matcher = m.selectedMatcher.matcherName()
	}
	return out
}

func sidecarFillSelectedWordStats(out *SidecarSelectedContext, stats *WordStats) {
	if stats == nil {
		return
	}
	out.FirstSource = sidecarLineSource(stats.source, true)
	out.FirstIntervalSource = sidecarLineSourceFromLogLine(stats.firstIntervalLogLine)
	out.LastSource = sidecarLineSourceFromLogLine(stats.lastLogLine)
}

func sidecarLineSourceFromLogLine(logLine string) *SidecarLineSource {
	if logLine == "" {
		return nil
	}

	out := &SidecarLineSource{
		LogLine:     logLine,
		MarkedState: SidecarMarkedStateUnmarked,
	}
	spaceIndex := strings.IndexByte(logLine, ' ')
	if spaceIndex != -1 {
		out.IP = logLine[:spaceIndex]
		if _, prefix := isLikelyIPv4AndPrefix(out.IP); prefix != "" {
			out.IPPrefix = prefix
		}
	}

	request, resp, bytesText, referer, agent, err := splitLogLineParts(logLine)
	if err != nil {
		return out
	}
	out.Request = request
	out.ResponseCode = resp
	out.Referer = referer
	out.UserAgent = agent
	if bytesValue, err := strconv.Atoi(bytesText); err == nil {
		out.BytesValue = bytesValue
	}
	return out
}

func sidecarLineSource(line *lineSource, includeLogLine bool) *SidecarLineSource {
	if line == nil || !includeLogLine {
		return nil
	}
	markedState, markedByMatcher := sidecarMarkedFields(line)
	out := &SidecarLineSource{
		IP:              line.ip,
		IPPrefix:        line.ipPrefix,
		Request:         line.request,
		Referer:         line.referer,
		BytesValue:      line.bytesValue,
		UserAgent:       line.userAgent,
		ResponseCode:    line.respCode,
		UserAgentDelta:  line.userAgentDelta,
		CaptureColor:    strings.Trim(line.captureColor, "[]"),
		MarkedState:     markedState,
		MarkedByMatcher: markedByMatcher,
		TokenBandCount:  line.tokenBandCount,
		UserAgentTokens: copyStringSlice(line.userAgentTokens),
	}
	if includeLogLine {
		out.LogLine = line.logLine
	}
	return out
}

func sidecarMarkedFields(line *lineSource) (string, string) {
	if line != nil && line.captureColor != "" {
		return SidecarMarkedStateMarked, line.captureMatcher
	}
	return SidecarMarkedStateUnmarked, ""
}

func copyIntSlice(in []int) []int {
	if len(in) == 0 {
		return nil
	}
	out := make([]int, len(in))
	copy(out, in)
	return out
}

func copyStringSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func copyUint64Map(in map[int]uint64) map[int]uint64 {
	if len(in) == 0 {
		return nil
	}
	out := make(map[int]uint64, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func sumIntSlice(in []int) int {
	total := 0
	for _, value := range in {
		total += value
	}
	return total
}

func sidecarEventTime(m *Monitor) time.Time {
	if m != nil && !m.logtime.IsZero() {
		return m.logtime
	}
	return time.Now()
}

func sidecarSessionFilename() string {
	return timestampedFilename(defaultSidecarPrefix, sidecarSessionID, defaultSidecarSuffix)
}

func newSidecarSessionID() string {
	return timestampedFileID(time.Now())
}

func sidecarUptimeSeconds() int64 {
	if startTime.IsZero() {
		return 0
	}
	return int64(time.Since(startTime).Seconds())
}

func sidecarHelpText() string {
	return `
PattyGraph JSONL output is intended for file-based automation consumers.
It leaves the normal TUI running, but also writes structured interval records
that can be tailed, replayed, or used to choose targeted raw-log searches.

Enable JSONL output from the CLI:
  pattyGraph -j 
  pattyGraph --json
  pattyGraph --json-file current.jsonl

Recommended quick AI startup pattern to start and read the last 10M of log file:
  pattyGraph --json --read 10 --config splats/pattyGraph.conf /path/to/access.log

The -r/--read option preloads recent log data before live tailing starts.
The startup replay pushes are also written to the pattyLog file, so a reader
gets immediate context before monitoring the live stream. Smaller read values
keep startup output bounded while still showing recent traffic shape.

Use --json-file to write a stable, specific PattyLog file instead of the default
pattyLog_<date>_<time>_<pid>.jsonl session filename. Relative paths are created
relative to <save-dir> when save-dir is configured; otherwise they use the
current working directory. A configured json-file is created/truncated at
session start.

Default output path:
  <save-dir>/pattyLog_<date>_<time>_<pid>.jsonl

If save-dir is not configured, the file is written in the current directory.
Without --json-file, each PattyGraph process gets a new pattyLog filename.
Existing timestamped pattyLog files are not cleared or appended across process
sessions. With --json-file, the configured file is created/truncated at session
start and then appended for the rest of that session.

JSONL Record types:
  session_start
      First record in the file. Contains process/session metadata, CLI args,
      output_path, file_path, machine name, and session_id. Its timestamp is
      wall-clock process startup time.

  interval
      One completed push interval. Contains core matcher counts, top words,
      top refs, top IPs, IP groups, factoids, summary metrics, and totals.
      Its timestamp is the log-derived time PattyGraph read from the access log,
      not wall-clock process time.

  control_command
      Immediate acknowledgement for a command consumed from pattyControl.log.
      Contains command text, source, status, control_file_enabled,
      control_file_path, and structured result metadata.

  alert
      Matcher alert transition. Written when a configured matcher alert bound
      is triggered or recovered. Manual alert configuration changes are recorded
      as control_command events.

Important fields:
  schema_version   Version of the pattyLog JSON schema.
  event_type       session_start, interval, control_command, or alert.
  session_id       Stable id shared by all records in one file.
  timestamp        Wall time for session_start; log time for interval records.
  interval         PattyGraph interval number.
  interval_lines   Log lines seen in this interval.
  matchers         Core matcher summaries such as Googlebot, bingbot, Bots,
                   lines, bytes, and errs.
  interesting      Ranked words, refs, and ips.
  factoids         Short generated observations about current state.

The pattyLog stream is not intended to replace grep, rg, awk, or direct log
inspection. It is a triage layer. Use it to answer where to start, then run
targeted raw-log searches based on the top IPs, prefixes, words, refs, bots,
and error codes.

Example follow-up searches after reading pattyLog output:
  rg " 404 " access.log
  rg "Googlebot|bingbot|Applebot" access.log
  rg "^192\\.0\\.|^198\\.51\\.|^203\\.0\\." access.log
  rg "GET /image|GET /filter" access.log

Notes:
  - CLI --json/-j controls pattyLog JSONL output.
  - The TUI remains active; pattyLog output is currently layered onto the same
    push-cycle timing.
`
}

func aiHelpText() string {
	return `
AI operation

PattyGraph is a live terminal/TUI instrument for NGINX access-log analysis.
For AI-assisted operation, run PattyGraph inside tmux so a human and an AI 
can operate through the same terminal session

Recommended shared startup pattern:

  tmux new-session -d -s pattygraph-ai './pattyGraph --json --control <normal args here>'

Human attach pattern:

  tmux attach -t pattygraph-ai

Recommended help topics to inspect before operating:

  ./pattyGraph --help
  ./pattyGraph --help json
  ./pattyGraph --help inline

Less important topics
  ./pattyGraph --help layout
  ./pattyGraph --help words
  ./pattyGraph --help facts

Important outputs

Live TUI:
  Human-facing operational view. It updates continuously and includes color,
  selection state, sparklines, changing counters, and short generated facts.
  Use it for shared human/AI situational awareness, not as the primary
  machine-readable data source.

pattySplat:
  Saved full text screen state. Use this for bounded AI analysis of the current
  display. A pattySplat includes the first 100 entries, not only the rows
  currently visible in the terminal.
  Force a pattySplat with the inline command:
    !!! pattySplat

pattyLog JSONL:
  Structured interval output. Use this for machine-readable state: interval
  summaries, matcher counts, interesting words/refs/IPs, selected-item source
  data, factoids, and inline command results.

  pattyLog files are saved in <save-dir> using:

    pattyLog_<date>_<time>_<pid>.jsonl

  The <date> and <time> fields are UTC:

    <date> = YYYYMMDD
    <time> = HHMMSS
    <pid>  = PattyGraph process id

Generated configs:
  PattyGraph can save matcher configuration snapshots using:

    pattyGraph_<date>_<time>_<pid>.conf

  These config files contain inline-command-style matcher definitions and can
  be reused with -config.

File discovery

All generated files are written to <save-dir>. If save-dir is not configured,
use the current working directory.

Find the latest pattySplat:

  ls -t <save-dir>/pattySplat_*.txt 2>/dev/null | head -1

Find the latest pattyLog JSONL file:

  ls -t <save-dir>/pattyLog_*.jsonl 2>/dev/null | head -1

Find the latest generated config:

  ls -t <save-dir>/pattyGraph_*.conf 2>/dev/null | head -1

Use a generated config directly:

  ./pattyGraph <normal args here> -config <latest-config>

If replacing a stable config such as pattyGraph.conf, make a backup first.

Do not use tmux capture-pane as the primary snapshot method. It only captures
visible terminal text. Prefer pattySplat for bounded screen-state analysis and
pattyLog JSONL for structured state.

Basic AI workflow

1. Start PattyGraph with --json and --control if the user has not already done
   so. Use --read with a bounded value when startup context is useful.
2. Locate the active pattyLog JSONL file and read the session_start record.
   Confirm output_path, file_path, control_file_enabled, and control_file_path.
3. Read recent interval records from the pattyLog file. Identify top words,
   refs, IPs, matcher counts, errors, IP groups, and factoids.
4. If display context matters, issue "!!! pattySplat" and inspect the latest
   pattySplat file instead of scraping tmux.
5. For deeper evidence, use targeted raw-log searches based on pattyLog
   findings. Avoid broad ingestion of large access logs unless the human asks.
6. If you change matchers or selections, wait for the next interval before
   judging the result.
7. Save a config if the matcher setup should persist.

Inline command workflow

Inline commands begin with:

  !!!

Use inline commands through pattyControl.log when --control is enabled, or in
config files before data ingestion. See '--help inline' for the full command set.

To issue an inline command:
Example inline command issued to create a pattySplat:
  printf '%s\n' '!!! pattySplat' >> <control-file-path>
Example inline command to select a specific IP:
  printf '%s\n' '!!! select --ips 203.0.113.10' >> <control-file-path>

Selection workflow:

1. Select an interesting item:

     !!! select --refs some_token
     !!! select --words some_token
     !!! select --ips 203.0.113.10
     # or to clear selection
     !!! select

2. Read the pattyLog control_command record to confirm the command was applied.
3. Read the next interval record for selected context and source-line metadata.

Matcher workflow:

1. Add, delete, color, and tune matchers using documented inline commands or
   TUI keys.
2. After matcher changes, wait for the next interval before judging results.
3. Save the generated config if the matcher setup should persist.

Example targeted raw-log searches

Use pattyLog JSONL to decide what to search for, then search the raw access log
directly. Examples:

  rg " 404 " access.log
  rg "Googlebot|bingbot|Applebot" access.log
  rg "^192\\.0\\.|^198\\.51\\.|^203\\.0\\." access.log
  rg "GET /image|GET /filter" access.log

Interval considerations:
Live TUI:
  Human-facing operational view. It updates once per second and includes color,
  selection state, sparklines, changing counters, and short generated facts.
pattyLog JSONL:
  Structured output written once per minute.

Interpretation rules

State which source supports your conclusions:

  "from the latest pattySplat"
  "from the latest pattyLog interval"
  "from the live TUI"
  "from targeted raw-log searches"

Do not claim second-by-second behavior unless you observed the live TUI or have
records at that resolution. JSONL pattyLog interval timestamps are log-derived for
interval records and wall-clock for session_start/control_command records.

Do not paste or ingest large raw logs unless the human specifically asks.
Prefer small, targeted searches based on PattyGraph findings.

Be conservative with destructive or persistent actions. Before replacing a
stable config, deleting matchers, or changing long-lived files, preserve a
backup or ask the human.
`
}
