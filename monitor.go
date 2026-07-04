// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/nxadm/tail"
	"github.com/rivo/tview"
)

// Monitor owns PattyGraph's live run state, display surfaces, matcher lanes,
// selected rows, and interval counters. The process uses one global Monitor
// instance, PattyGraph, so runtime settings and UI state flow through this
// structure.
//
// Matcher order is meaningful. Conceptually, each parsed log line is dropped
// through an ordered stack of matcher sieves. A row may add semantics to
// currentLine as the line passes, but lower rows do not reach back upward to
// change earlier decisions.
//
// Rows above Bots are competitive: at most one row claims a log line before
// Bots sees it. Bots is the boundary row for unclaimed bot-family traffic. Rows
// below Bots are observational: they see the parsed line after the competitive
// phase and add system counts plus interesting words, refs, and IPs.
//
// The same order drives TUI row layout, inline matcher insertion, bot/IP
// promotion, remembered-IP tagging, and historical sparkline scaling. Matchers
// above Bots plus Bots itself share global historical scale; rows below Bots
// generally use local scale and do not compete for ownership of the line.
type Monitor struct {
	pattyConfig *MonitorConfig

	filePath           string // access log being monitored
	totalLines         uint64 // lines seen since startup
	totalBytes         uint64 // bytes seen since startup
	intervalLines      int    // lines seen during the current interval
	intervalsCompleted int    // completed intervals since startup

	app                  *tview.Application
	layout               *tview.Flex
	sparklineHistoryView *tview.TextView // matcher list, sparklines, and selected history
	botMatchesView       *tview.TextView // selected matcher detail
	wordMatchesView      *tview.TextView // interesting word results
	refsView             *tview.TextView // interesting referrer results
	ipsView              *tview.TextView // interesting IP results

	// Core matcher lanes kept by name for fast access and UI coordination.
	matchers     []MatcherFacade
	botsMatcher  *Matcher
	wordsMatcher *InterestingWordMatcher
	refsMatcher  *InterestingWordMatcher
	ipsMatcher   *InterestingWordMatcher
	linesMatcher *Matcher
	bytesMatcher *Matcher
	errsMatcher  *Matcher

	tabViewIndexKey    int // secondary-view tab index
	selectedGraphValue int
	logtime            time.Time  // updated once a cycle from log input. Used for status display only.
	logtimeCache       *time.Time // part of the trigger for log time cache update once a cycle
	//selectedGraphPosition string  // used for debugging mouse click x.y capture

	selectedInterestingMatcher *InterestingWordMatcher
	selectedMatcher            *Matcher
	selectionValue             string
	miniWindowIndex            int

	overallMax, overallMin int
	demo                   bool
	showTicker             bool
	// Runtime metrics and alert transitions.
	totalAgentTokens        uint64
	unmarked                int
	pendingAlertTransitions []AlertTransition
}

// PattyGraph's process-wide runtime anchors are grouped here because they are
// shared by the access-log reader, control-file reader, matcher pipeline, and
// TUI. PattyGraph is a terminal tool, not a reusable library; these globals make
// the hot path direct while keeping the intentional shared state visible.
var PattyGraph *Monitor

var mu sync.RWMutex             // shared model lock across readers and TUI updates
var currentLine = &lineSource{} // line currently being processed; only valid during a match cycle

// Cycle counters are package-level because parsing, matching, and display all
// read the same log-time position during the hot path.
var currentCycle int  // current cycle number counting up to DefaultIntervalSize
var logicalCycles int // completed or skipped cycles since startup

// botsIndex is the matcher ordering boundary. Rows above Bots compete for a
// line, Bots receives unclaimed bot-family traffic, and rows below Bots observe
// the line without claiming it.
var botsIndex = -1

var uaCardinalityMap = make(map[int]uint64, 20)
var totalAgentTokenCount uint64

// MatcherFacade is the shared lane interface for PattyGraph's ordered TUI rows.
//
// The visible row order contains two different kinds of objects:
//   - Matcher rows, which can claim log lines and participate in bot competition.
//   - Interesting streams, which observe every line after Bots and expose words,
//     refs, and IPs.
//
// Keeping them in one ordered slice lets the UI, push cycle, config output, and
// row insertion logic operate on the same visual lane model. Some methods only
// have meaningful behavior for concrete Matcher rows, so callers that need
// Matcher-specific state must use asMatcher() and handle nil for interesting
// streams.
type MatcherFacade interface {
	push()               // Pushes data to the matcher
	match() bool         // Matches against currentLine
	matcherName() string // Provides the matcher’s name
	displayString() string
	displayMatched() string // Generates a display string
	setColor(color string)
	minMaxHistory() (int, int)
	getCount() int
	asMatcher() *Matcher
	asInlineCommand() string
}

// lineSource is the parsed, shared view of the current log line. Parsing,
// normalization, and tokenization happen once per line so matchers and
// interesting streams can reuse the same fields without redoing hot-path work.
//
// Consumers should treat set fields as immutable during a match cycle. Simple
// matchers may attach capture color/name metadata, and word matchers retain only
// selected source examples instead of copying every line.
type lineSource struct {
	logLine        string
	ip             string
	ipPrefix       string
	request        string
	referer        string
	bytesValue     int
	userAgent      string
	respCode       string
	userAgentDelta float64 // The delta seen per already seen IP request
	captureColor   string  // color assigned by the matcher that claimed this line
	captureMatcher string

	tokenBandCount int

	// Matching-only derived fields; rebuilt for each parsed log line.
	userAgentTokens   []string
	replacedUserAgent string
	cloned            *lineSource
}

func (l lineSource) isError() bool {
	if l.respCode == "" {
		return false
	}
	if (l.respCode)[0] == '4' || (l.respCode)[0] == '5' {
		return true
	}
	return false
}

func NewMonitor(conf *MonitorConfig) *Monitor {
	filePath := conf.filePath
	var matchers []MatcherFacade

	botsMatcher := MatcherFactory("Bots")
	matchers = append(matchers, botsMatcher)
	linesMatcher := MatcherFactory("lines")
	matchers = append(matchers, linesMatcher)
	bytesMatcher := MatcherFactory("bytes")
	matchers = append(matchers, bytesMatcher)
	errsMatcher := MatcherFactory("errs")
	matchers = append(matchers, errsMatcher)

	words := WordMatcherFactory("words")
	matchers = append(matchers, words)
	refs := WordMatcherFactory("refs")
	matchers = append(matchers, refs)
	ips := WordMatcherFactory("ips")
	matchers = append(matchers, ips)

	return &Monitor{
		pattyConfig:          conf,
		filePath:             filePath,
		matchers:             matchers,
		botsMatcher:          botsMatcher,
		linesMatcher:         linesMatcher,
		bytesMatcher:         bytesMatcher,
		errsMatcher:          errsMatcher,
		wordsMatcher:         words,
		refsMatcher:          refs,
		ipsMatcher:           ips,
		app:                  tview.NewApplication(),
		sparklineHistoryView: createTextView(),
		botMatchesView:       createTextView(),
		wordMatchesView:      createTextView(),
		refsView:             createTextView(),
		ipsView:              createTextView(),
		showTicker:           true,
	}
}

func createTextView() *tview.TextView {
	return tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
}

// Expensive but done during push or cached on overallMax < 0 to invalidate (also on a push)
func (m *Monitor) minAvgMaxHistoryAcrossMatchers() (int, float64, int) {
	if len(m.matchers) == 0 {
		return 0, 0, 0
	}

	var filteredMatchers []MatcherFacade
	// Rows above Bots share historical graph scaling; system rows and
	// interesting streams below Bots use their own local scales.
	for i, matcher := range m.matchers {
		if i < botsIndex {
			filteredMatchers = append(filteredMatchers, matcher)
		}
	}
	if len(filteredMatchers) == 0 {
		return 0, 0, 0
	}

	// Initialize overallMin and overallMax with the values from the first matcher
	overallMin, overallMax := filteredMatchers[0].minMaxHistory()
	tMax := overallMax

	for _, matcher := range filteredMatchers[1:] { // Start from the second matcher
		minInMatcher, maxInMatcher := matcher.minMaxHistory()
		tMax += maxInMatcher

		if minInMatcher < overallMin {
			overallMin = minInMatcher
		}
		if maxInMatcher > overallMax {
			overallMax = maxInMatcher
		}
	}
	avgMax := float64(float64(tMax) / float64(len(filteredMatchers)))

	return overallMin, avgMax, overallMax
}

var lastMonitorMaxBuf = &ringBuffer{}
var lastLinesBuf = &ringBuffer{}
var lastBytesBuf = &ringBuffer{}

// calls all matcher.push within the Monitor
// signals the end of an interval (DefaultIntervalSize cycles completed). Interval based
// counters should be reset now
func push() {
	PattyGraph.overallMax = -1 // -1 is a cache invalidation signal that causes recomputation of values for displayString()
	_, avg, mmx := PattyGraph.minAvgMaxHistoryAcrossMatchers()
	lastMonitorMaxBuf.Push(mmx)
	_, linesMax := PattyGraph.linesMatcher.minMaxHistory()
	_, bytesMax := PattyGraph.bytesMatcher.minMaxHistory()
	lastLinesBuf.Push(linesMax)
	lastBytesBuf.Push(bytesMax)

	// inject new matchers here if needed
	if !PattyGraph.botsMatcher.disableAutoAdd {
		PattyGraph.botsMatcher.migrateBots(avg)

		// if we're first started, try to inject up to two more
		if PattyGraph.intervalsCompleted == 0 {
			botMin := avg
			if botMin == 0 { // happens at start
				botMin = float64(PattyGraph.botsMatcher.getCount() / 4)
			}
			PattyGraph.botsMatcher.migrateBots(botMin)
			PattyGraph.botsMatcher.migrateBots(botMin)
		}
	}

	// Do the actual push
	for i := range PattyGraph.matchers {
		PattyGraph.matchers[i].push()
	}
	// check if any one ip is exceeding a max lines highwater mark
	if PattyGraph.intervalsCompleted >= pattyGracePeriod {
		PattyGraph.ipsMatcher.migrateIps()
	}
	if generateJsonSparks && PattyGraph.layout != nil {
		PattyGraph.writeSparklineJSON()
	}
	botsIndex = botsMatcherIndex() // make sure its always in check, used to be done per-logline!
	// reset lines counted this interval
	PattyGraph.logtimeCache = nil
	PattyGraph.intervalLines = 0
	PattyGraph.intervalsCompleted++
	PattyGraph.unmarked = 0
}

func (m *Monitor) registerAlertTransition(t AlertTransition) {
	if m == nil {
		return
	}
	m.pendingAlertTransitions = append(m.pendingAlertTransitions, t)
}

func (m *Monitor) writePendingAlertTransitionsJSONL() {
	if m == nil || !generateSidecarJSONL {
		return
	}
	for _, transition := range m.pendingAlertTransitions {
		if err := m.WriteSidecarAlertJSONL(transition, ""); err != nil {
			log.Printf("PattyLog alert jsonl write failed: %v", err)
		}
	}
}

func (m *Monitor) clearPendingAlertTransitions() {
	if m == nil {
		return
	}
	m.pendingAlertTransitions = m.pendingAlertTransitions[:0]
}

func renderProgressBar() string {
	// progressBarWidth is always 15
	// DefaultIntervalSize is always 60
	// these used to be changeable but not any longer,
	// still keeping them factored out because it keeps things cleaner
	// Integer division: map currentCycle [0–60] → [0–15]
	filledBars := currentCycle * progressBarWidth / DefaultIntervalSize
	return strings.Repeat("=", filledBars) + strings.Repeat("-", progressBarWidth-filledBars)
}

// match parses the string into a lineSource and iterates over all matchers
// this used to be all regex but is now as much manual parsing as possible
// to avoid backtrack regex parsing
func match(line string) {
	PattyGraph.intervalLines++
	PattyGraph.totalLines++
	poolGetsStart := poolGets

	// initially, one regex got everything but its a manual parse now to
	// avoid overhead since it can be done manually
	//
	// Performing basic string match for the IP bc its predictable and cheap.
	// Find the first space, which separates the IP from the rest of the log logLine
	spaceIndex := strings.Index(line, " ")
	if spaceIndex == -1 {
		// If no spaces came in it can't be a valid log logLine,
		return
	}

	// the '!!! cmd...' inline invocation
	if 3 == spaceIndex && InlinePreamble == line[:3] {
		invokeInlineCommand(line)
		return
	}

	ipTmp := stringInterner.Intern(line[:spaceIndex])
	ok, prefix := isLikelyIPv4AndPrefix(ipTmp)
	// Validate if it's a proper IP address
	if !ok {
		return
	}

	// parse the time manually to avoid regex
	var err error
	if PattyGraph.logtimeCache == nil {
		PattyGraph.logtimeCache, err = extractTimestamp(line)
		if err != nil {
			return
		}
	}
	PattyGraph.logtime = *PattyGraph.logtimeCache
	*currentLine = lineSource{
		ip:       ipTmp,
		ipPrefix: prefix,
		logLine:  line,
	}

	err = splitLogLinePartsIntoCurrent()
	//req, respCode, bytesStr, ref, ua, err := splitLogLineParts(line)
	if err != nil {
		return
	}

	// This gets used by user-agent distance computing in ips. Doing it here early since its always done and others
	// might benefit from the userAgentDelta scale computation.
	currentLine.replacedUserAgent = symbolReplacer.Replace(currentLine.userAgent)
	tokens := fastFieldsASCIIBuf(currentLine.replacedUserAgent, &uaFieldsBuf)
	currentLine.userAgentTokens = stringInterner.InternAll(tokens)
	tokenBandCount := len(currentLine.userAgentTokens)
	uaCardinalityMap[tokenBandCount]++
	totalAgentTokenCount = totalAgentTokenCount + uint64(tokenBandCount)
	//currentLine.tokenBand = classifyTokenCount(tokenBandCount)
	currentLine.tokenBandCount = tokenBandCount

	PattyGraph.totalBytes += uint64(currentLine.bytesValue)
	/************************/
	/* logLine parsing is done
	/* sourceLineInput logLine data will not change other than data striping like color getting set
	/* by a simple matcher if matched
	/************************/
	if prevStats, exists := PattyGraph.ipsMatcher.wordFrequency[ipTmp]; exists {
		// only compute the difference if one exists
		if prevStats.source.userAgent != currentLine.userAgent {
			currentLine.userAgentDelta = levenshteinTokensRatio(prevStats.agentTokensFromSource, currentLine.userAgentTokens)
		}
	}
	// Remembered-IP tagging runs before bot competition so promoted matchers above
	// Bots can mark traffic they previously learned, even if the current line no
	// longer carries the original identifying User-Agent.
	for i, matcher := range PattyGraph.matchers {
		if i < botsIndex {
			basicMatcher := matcher.asMatcher() // no interesting matchers should be here
			if basicMatcher.tagIpAction() {
				break
			}
			//if matcherColor, exists := basicMatcher.tagIp(currentLine.ip); exists {
			//	currentLine.captureColor = matcherColor
			//	break
			//}
		}
	}
	// Competitive phase: rows above Bots and Bots itself get first claim.
	// A successful match stops this phase; reaching Bots also stops it.
	for i, matcher := range PattyGraph.matchers {
		if matcher.match() {
			break
		}
		if i == botsIndex {
			break
		}
	}
	// Observational phase: rows below Bots all see the line, including system
	// counters, interesting streams, and matchers intentionally placed below Bots.
	for i, matcher := range PattyGraph.matchers {
		if i > botsIndex {
			matcher.match()
		}
	}
	poolGetsStop := poolGets
	poolGetsThisCall := poolGetsStop - poolGetsStart
	poolGetsMap[int(poolGetsThisCall)]++
}

func initCycle() {
	if !forceZeroStart {
		currentCycle = max(0, time.Now().Second()-1)
	}
}

func resetCycle() {
	currentCycle = 0
}

func incrementCycle() {
	currentCycle++
	//cycleTime = time.Now()
	PattyGraph.logtimeCache = nil
	logicalCycles++
	if PattyGraph.demo {
		if logicalCycles%10 == 0 {
			PattyGraph.tabViewIndexKey = (PattyGraph.tabViewIndexKey + 1) % SecondaryInfoTabDepth
		}
	}
	if displayFreezeCountdown > 0 {
		displayFreezeCountdown--
	}
}

var lineCh chan string
var controlFileMonitorStarted bool

func controlFileStartMarker() string {
	return fmt.Sprintf("# pattyGraph control ready session_id=%s control_file_enabled=%t file_path=%q sidecar_path=%q", sidecarSessionID, enableControlFile, PattyGraph.filePath, PattyGraph.SidecarDefaultPath())
}

func controlFilePath() string {
	if PattyGraph.pattyConfig.saveDir != "" {
		return filepath.Join(PattyGraph.pattyConfig.saveDir, "pattyControl.log")
	}
	return "pattyControl.log"
}

func parseControlEnabled(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "off", "false", "0", "no":
		return false
	default:
		return true
	}
}

func isControlEnableLine(text string) bool {
	if !strings.HasPrefix(text, InlinePreamble+" ") {
		return false
	}
	fields := strings.Fields(text)
	if len(fields) < 2 || strings.ToLower(fields[1]) != "control" {
		return false
	}
	if len(fields) == 2 {
		return true
	}
	return parseControlEnabled(fields[2])
}

func shouldProcessControlLine(text string) bool {
	if !strings.HasPrefix(text, InlinePreamble+" ") {
		return false
	}
	return enableControlFile || isControlEnableLine(text)
}

func startControlFileMonitoring() {
	if controlFileMonitorStarted {
		return
	}
	path := controlFilePath()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("Failed to create control file %s: %v", path, err)
		return
	}
	if _, err := fmt.Fprintln(f, controlFileStartMarker()); err != nil {
		log.Printf("Failed to write control file marker %s: %v", path, err)
		_ = f.Close()
		return
	}
	if err := f.Close(); err != nil {
		log.Printf("Failed to close control file %s: %v", path, err)
		return
	}

	t, err := tail.TailFile(path, tail.Config{
		Follow:   true,
		ReOpen:   true,
		Logger:   tail.DiscardingLogger,
		Location: &tail.SeekInfo{Offset: 0, Whence: io.SeekEnd},
	})
	if err != nil {
		log.Printf("Failed to monitor control file %s: %v", path, err)
		return
	}
	controlFileMonitorStarted = true

	go func() {
		for line := range t.Lines {
			text := strings.TrimRight(line.Text, "\r\n")
			if !shouldProcessControlLine(text) {
				continue
			}
			mu.Lock()
			result := invokeInlineCommand(text)
			if generateSidecarJSONL {
				if err := PattyGraph.WriteSidecarControlCommandJSONL(text, "control_file", result, ""); err != nil {
					log.Printf("PattyLog control command write failed: %v", err)
				}
			}
			mu.Unlock()
		}
	}()
}

func startFileMonitoring() {
	t, err := tail.TailFile(PattyGraph.filePath, tail.Config{
		Follow:   true,
		ReOpen:   true,
		Logger:   tail.DiscardingLogger,
		Location: &tail.SeekInfo{Offset: 0, Whence: io.SeekEnd}, // Start at end of file
	})
	if err != nil {
		log.Fatalf("Failed to open file: %v\n", err)
	}

	// Channel to pass lines for processing
	lineCh = make(chan string, 200) // Buffered channel to prevent blocking

	// Goroutine to read lines and push to channel
	go func() {
		for line := range t.Lines {
			lineCh <- line.Text
		}
		close(lineCh) // Close the channel when file reading ends
	}()

	// Goroutine to process lines from the channel
	go func() {
		gcCount := 0
		for line := range lineCh {
			mu.Lock()   // Write-protect match
			match(line) // Check all matchers for the current logLine
			if gcCount >= 100000 {
				compactCaches()
				gcCount = 0
			}
			mu.Unlock()
			gcCount++
		}
	}()
}

func compactCaches() {
	PattyGraph.wordsMatcher.compactFrequencyMap()
	PattyGraph.refsMatcher.compactFrequencyMap()
	PattyGraph.ipsMatcher.compactFrequencyMap()
	stringInterner.compactLRU()
}
func expandUser(path string) string {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[1:])
		}
	}
	return path
}

func (m *Monitor) printToFile() error {
	filename := newTimestampedFilename("pattySplat_", ".txt")
	fullPath := filename
	if m.pattyConfig.saveDir != "" {
		fullPath = filepath.Join(m.pattyConfig.saveDir, filename)
	}
	file, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer file.Close()

	text := m.sparklineHistoryView.GetText(true)
	_, err = file.WriteString(text)
	if err != nil {
		log.Fatalf("Error writing to file: %v", err)
	}
	err = WriteColumnsToFile(file,
		m.botMatchesView.GetText(true),
		m.wordMatchesView.GetText(true),
		m.refsView.GetText(true),
		m.ipsView.GetText(true))
	if err != nil {
		log.Fatalf("Error writing to file: %v", err)
	}
	return nil
}

func (m *Monitor) purgeAllPeakContent() {
	if m.selectedMatcher != nil {
		//if m.selectedMatcher.isAddedAutobot {
		m.selectedMatcher.purgeMatchedWords()
		//}
		return
	}
	purgePeakWordCommand()
}
func (m *Monitor) deleteSelectedMatcher() {
	if m.selectedMatcher == nil ||
		//m.selectedMatcher == PattyGraph.botsMatcher ||
		m.selectedMatcher == PattyGraph.bytesMatcher ||
		m.selectedMatcher == PattyGraph.linesMatcher ||
		m.selectedMatcher == PattyGraph.errsMatcher {
		return
	}
	removeMatcher(m.selectedMatcher.matcherName())
	UpdateHistoricFlags()
}

func (m *Monitor) playConfigFile(configFile string) {
	//m.pattyConfig.builtinConfFile
	if configFile == "" {
		//log.Printf("No config file specified")
		return
	}

	f, err := os.Open(configFile)
	if err != nil {
		log.Printf("Failed to open config file %s: %v", configFile, err)
		return
	}
	defer f.Close()

	// Read all lines into a slice
	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, InlinePreamble+" ") {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("Error reading config file %s: %v", configFile, err)
	}

	for _, line := range lines {
		invokeInlineCommand(line)
	}
}

func matcherSelectionColor() string {
	if PattyGraph.selectedMatcher == nil {
		return ""
	}
	if !PattyGraph.selectedMatcher.isAddedAutobot && PattyGraph.selectedMatcher != PattyGraph.botsMatcher {
		return ""
	}
	return PattyGraph.selectedMatcher.color[1 : len(PattyGraph.selectedMatcher.color)-1]
}

func WriteColumnsToFile(file *os.File, col1, col2, col3, col4 string) error {
	// Split the text views into lines
	lines1 := strings.Split(col1, "\n")
	lines2 := strings.Split(col2, "\n")
	lines3 := strings.Split(col3, "\n")
	lines4 := strings.Split(col4, "\n")

	// Determine the maximum number of lines across all views
	maxLinesToPrint := maxOfFour(len(lines1), len(lines2), len(lines3), len(lines4))

	// Define fixed column widths
	widths := []int{26, 26, 26, 22}

	// Build each row in parallel
	for i := 0; i < maxLinesToPrint; i++ {
		col1Text := sanitize(truncate(getLineOrBlank(lines1, i), widths[0]))
		col2Text := sanitize(truncate(getLineOrBlank(lines2, i), widths[1]))
		col3Text := sanitize(truncate(getLineOrBlank(lines3, i), widths[2]))
		col4Text := sanitize(truncate(getLineOrBlank(lines4, i), widths[3]))

		// Format each column to its width
		row := fmt.Sprintf("%-*s%-*s%-*s%-*s\n",
			widths[0], col1Text,
			widths[1], col2Text,
			widths[2], col3Text,
			widths[3], col4Text)

		// Write the row to the file
		_, err := file.WriteString(row)
		if err != nil {
			return err
		}
	}
	return nil
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' {
			b.WriteRune(r)
		} else if unicode.IsPrint(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune('?') // Replace with ? or your fallback
		}
	}
	return b.String()
}
func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	// If we have room for an ellipsis...
	if max > 1 {
		return string(runes[:max-1]) + "…"
	}
	return string(runes[:max])
}

func truncateOld(s string, max int) string {
	if len(s) <= max {
		return s
	}
	// Optional: add ellipsis or just cut cleanly
	if max > 1 {
		return s[:max-1] + "…"
	}
	return s[:max]
}

// Helper function to get a logLine or return a blank if out of range
func getLineOrBlank(lines []string, index int) string {
	if index < len(lines) {
		return lines[index]
	}
	return ""
}

// Helper function to get the maximum of four integers
func maxOfFour(a, b, c, d int) int {
	if a > b {
		b = a
	}
	if c > d {
		d = c
	}
	if b > d {
		return b
	}
	return d
}

func splitArgsShellStyle(input string) ([]string, error) {
	var args []string
	var current strings.Builder
	inQuotes := false

	for i := 0; i < len(input); i++ {
		c := input[i]
		switch c {
		case ' ':
			if inQuotes {
				current.WriteByte(c)
			} else if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		case '"', '\'':
			inQuotes = !inQuotes
		default:
			current.WriteByte(c)
		}
	}

	if current.Len() > 0 {
		args = append(args, current.String())
	}
	if inQuotes {
		return nil, fmt.Errorf("unclosed quote in input")
	}

	// Strip surrounding quotes
	for i, arg := range args {
		if len(arg) >= 2 {
			first, last := arg[0], arg[len(arg)-1]
			if (first == '\'' && last == '\'') || (first == '"' && last == '"') {
				args[i] = arg[1 : len(arg)-1]
			}
		}
	}

	return args, nil
}

var facts = NewFactoidGenerator()

var (
	tickerBuffer        string
	tickerVisibleOffset int // scroll position, in visible characters
)
