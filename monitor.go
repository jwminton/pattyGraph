// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/nxadm/tail"
	"github.com/rivo/tview"
)

// TODO: tighten this up or nuke it
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

// Every logLine is put into this in one place as efficiently as possible for everyone else's reuse
// No line altering (e.g. toLower) or tokenization should be repeated.
// Not enforced but the design intent is that once a field is set it acts as an immutable structure.
// No consumer should be changing a set field.
// Simple matchers with a color set will transfer the last (or first if configured) match color to this
// Pass by reference only
// Word matchers only keep first and last for the source lines
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
	captureColor   string  // could probably replace with a data striping approach
	captureMatcher string

	//tokenBand AgentBand
	tokenBandCount int

	// caches only needed while matching... can be nuked post match()
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

// TODO Maybe move these back to being part of monitor and accessed as PattyGraph.currentCycle
var currentCycle int  // the current cycle number counting UP to DefaultIntervalSize
var logicalCycles int // Number of cycles real (or skipped by reading previous lines on startup)

// Monitor
/**
There's really only ever one monitor and its set as the global PattyGraph. Global settings should try to go through that.

Matchers are currently layered like this and their properties:
<IP_autobots>    	Simple				historical
<conf_file_bots> 	Simple				historical
<autobots>			Simple				historical
<Bots>				NOT Simple(Regex)	historical
<lines>				Simple (Lines)		NOT historical
<bytes> 			Simple (bytes) 		NOT historical
<words>  	Not Simple (WordMatcher)	NOT historical
<refs>  	Not Simple (WordMatcher)	NOT historical
<ips>  		Not Simple (WordMatcher)	NOT historical

Word matchers return "" unless they have a selection then their displayString() is used to show the spark info and
logline at the bottom of the sparkgraph pane.

IP_autobots are added by Ips when 10% of lines comes from 1 IP and they're added to the top of the list
autobots are added by Bots when its threshold is crossed
Matching starts at the top and proceeds to Bots first. If any of thost "match", none of the other see the match attempt.
After that, all matches after bots get a chance at the match.

This grew organically so it's all over the place. I'm still finding surprises.
*/
// TODO Review this, its grown without consideration. There's only one monitor and there's been no distinction between
// what's global and what's in the monitor

var mu sync.RWMutex             // Add RWMutex for synchronization
var currentLine = &lineSource{} // line currently being processed (only valid from match cycle)
var uaCardinalityMap = make(map[int]uint64, 20)
var totalAgentTokenCount uint64

type Monitor struct {
	pattyConfig *MonitorConfig

	filePath           string // file being  monitored
	totalLines         uint64 // total number of lines seen since started
	totalBytes         uint64 // total number of bytes totaled since started
	intervalLines      int    // total number of lines seen during this DefaultIntervalSize
	intervalsCompleted int    // total number of intervals completed

	app                  *tview.Application
	layout               *tview.Flex
	sparklineHistoryView *tview.TextView // For matcher list and history view
	botMatchesView       *tview.TextView // View for displayMatched() content of matchers
	wordMatchesView      *tview.TextView // For interesting word results
	refsView             *tview.TextView // For interesting urls results
	ipsView              *tview.TextView // For interesting ips results

	// Tracking well known matchers, finally
	matchers     []MatcherFacade
	botsMatcher  *Matcher
	wordsMatcher *InterestingWordMatcher
	refsMatcher  *InterestingWordMatcher
	ipsMatcher   *InterestingWordMatcher
	linesMatcher *Matcher
	bytesMatcher *Matcher
	errsMatcher  *Matcher

	tabViewIndexKey    int // tab view index
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
	// metrics
	totalAgentTokens        uint64
	unmarked                int
	pendingAlertTransitions []AlertTransition
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

	// Slice to hold matchers with includeHistory == true
	var filteredMatchers []MatcherFacade
	//botsIndex = botsMatcherIndex()
	// TODO This can benefit from a Historic marker interface for Matcher
	// Collect only matchers where includeHistory is true
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

// splits at the timestamp/request border. The request is the first quoted field
func secondPartOnly(line string) (string, error) {
	// Find the index of the first quote (start of the request)
	quoteIndex := strings.Index(line, "\"")
	if quoteIndex == -1 {
		return "", fmt.Errorf("log logLine does not contain a valid request")
	}

	// Split into two substrings
	//firstPart := strings.TrimSpace(logLine[:quoteIndex])  // From start to before the first quote
	secondPart := strings.TrimSpace(line[quoteIndex:]) // From the first quote onward

	return secondPart, nil
}

func findQuoteIndexes(line string) ([5]int, error) {
	var indexes [5]int
	count := 0

	for i := 0; i < len(line); i++ {
		if line[i] == '"' {
			if count < 5 {
				indexes[count] = i
				count++
				if count == 5 {
					break
				}
			}
		}
	}

	if count < 5 {
		return [5]int{}, fmt.Errorf("not enough quotes found, expected 5, got %d", count)
	}

	return indexes, nil
}

func findUserAgentCloseQuote(line string, userAgentOpenQuote int) (int, error) {
	end := strings.TrimRightFunc(line, unicode.IsSpace)
	if len(end) <= userAgentOpenQuote+1 || end[len(end)-1] != '"' {
		return 0, fmt.Errorf("log line missing user-agent closing quote")
	}

	if strings.HasSuffix(end, ` "-"`) {
		userAgentCloseQuote := len(end) - len(` "-"`) - 1
		if userAgentCloseQuote > userAgentOpenQuote {
			return userAgentCloseQuote, nil
		}
	}
	return len(end) - 1, nil
}

func splitLogLinePartsIntoCurrent() error {
	quoteIndex := strings.IndexByte(currentLine.logLine, '"')
	if quoteIndex == -1 {
		return fmt.Errorf("log line missing opening quote")
	}
	line := currentLine.logLine[quoteIndex:]
	quoteIndexes, err := findQuoteIndexes(line)
	if err != nil {
		return err
	}
	// Parse directly into fields
	//out.logLine = fullLine
	currentLine.request = line[quoteIndexes[0]+1 : quoteIndexes[1]]
	currentLine.respCode = line[quoteIndexes[1]+2 : quoteIndexes[1]+5]

	byteStr := line[quoteIndexes[1]+6 : quoteIndexes[2]-1]
	bytesVal, err := strconv.Atoi(byteStr)
	if err != nil {
		return fmt.Errorf("bad byte count: %w", err)
	}
	currentLine.bytesValue = bytesVal

	currentLine.referer = line[quoteIndexes[2]+1 : quoteIndexes[3]]
	userAgentCloseQuote, err := findUserAgentCloseQuote(line, quoteIndexes[4])
	if err != nil {
		return err
	}
	currentLine.userAgent = line[quoteIndexes[4]+1 : userAgentCloseQuote]

	return nil
}

// structured for efficiency in overall log line parsing
// splits out the 3 quoted strings and two ints in between as 5 returned strings.
func splitLogLineParts(fullLine string) (string, string, string, string, string, error) {
	quoteIndex := strings.Index(fullLine, "\"")
	if quoteIndex == -1 {
		return "", "", "", "", "", fmt.Errorf("log logLine does not contain a valid request")
	}

	// Split into two substrings
	//firstPart := strings.TrimSpace(logLine[:quoteIndex])  // From start to before the first quote
	line := strings.TrimSpace(fullLine[quoteIndex:]) // From the first quote onward

	// ^"GET /requested/url HTTP/1.1" 200 1234 "referer_url/text" "user agent text"
	// The first 5 double quotes are guaranteed to be present and delineate the request,
	// referer, and user-agent opening quote. The user-agent closing quote is found from
	// the right side so quote bytes inside user-agent content do not affect parsing.
	// Fields after the user-agent are ignored.
	// use these facts to avoid regex parsing
	quoteIndexes, err := findQuoteIndexes(line)
	if err != nil {
		return "", "", "", "", "", err
	}
	// The returned int slice did this:
	// ^"GET /requested/url HTTP/1.1" 200 1234 "referer_url/text" "user agent text"
	//  0                           1          2                3 4
	// Above are the quotesIndexes legend, no need to mentally juggle
	// messy but avoids regex and backtrack parsing
	request := line[quoteIndexes[0]+1 : quoteIndexes[1]]
	// response code is always 3 chars with a single space on either side
	resp := line[quoteIndexes[1]+2 : quoteIndexes[1]+5]
	// bytes returned bounds is a fixed distance from the request string end quote
	// and a fixed distance from the user agent start quote
	bytes := line[quoteIndexes[1]+6 : quoteIndexes[2]-1]
	referer := line[quoteIndexes[2]+1 : quoteIndexes[3]]
	userAgentCloseQuote, err := findUserAgentCloseQuote(line, quoteIndexes[4])
	if err != nil {
		return "", "", "", "", "", err
	}
	agent := line[quoteIndexes[4]+1 : userAgentCloseQuote]
	// no backtrack, no regex, log logLine parsing is an easy predictable pattern
	return request, resp, bytes, referer, agent, nil
}

var botsIndex = -1

// Executed once a cycle for status display only
func extractTimestamp(s string) (*time.Time, error) {
	start := strings.IndexByte(s, '[')
	if start == -1 || start+21 >= len(s) {
		return &time.Time{}, nil
	}
	// Layout expects 20 characters: "02/Jan/2006:15:04:05"
	timestampStr := s[start+1 : start+21] // safe slice
	t, err := time.ParseInLocation("02/Jan/2006:15:04:05", timestampStr, time.Local)
	return &t, err
}

// fast fields scratch areas. Each of these is call site specific to prevent clobbering while reusing the backing stores
// Each should have one usage each. Callers resize and reassign back into these slots!
var uaFieldsBuf []string = make([]string, 0, 50)
var refFieldsBuf []string = make([]string, 0, 50)
var botsFieldsBuf []string = make([]string, 0, 50)
var reqFieldsBuf []string = make([]string, 0, 50)

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
	// TODO MatcherFacade might be more annoying than its worth
	// grab autobot matcher color
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
	// let the autobots & Bots compete for the match
	for i, matcher := range PattyGraph.matchers {
		if matcher.match() {
			break
		}
		if i == botsIndex {
			break
		}
	}
	// Now the un-historic matchers (lines, bytes, errors, Interesting, -C additions) all get their match time
	for i, matcher := range PattyGraph.matchers {
		if i > botsIndex {
			matcher.match()
		}
	}
	poolGetsStop := poolGets
	poolGetsThisCall := poolGetsStop - poolGetsStart
	poolGetsMap[int(poolGetsThisCall)]++
}

const (
	InlineCommandStatusApplied  = "applied"
	InlineCommandStatusIgnored  = "ignored"
	InlineCommandStatusRejected = "rejected"
	InlineCommandStatusError    = "error"
)

type InlineCommandResult struct {
	CommandName string
	Status      string
	Result      map[string]interface{}
}

func inlineCommandResult(commandName string, status string, action string) InlineCommandResult {
	return InlineCommandResult{
		CommandName: strings.ToLower(commandName),
		Status:      status,
		Result:      map[string]interface{}{"action": action},
	}
}

func inlineCommandRejected(commandName string, action string, message string) InlineCommandResult {
	result := inlineCommandResult(commandName, InlineCommandStatusRejected, action)
	result.Result["error"] = message
	return result
}

func invokeInlineCommand(line string) InlineCommandResult {
	// Assume '!!! ' prefix already detected
	commandLine := line[4:]
	tokens := strings.Fields(commandLine)
	if len(tokens) == 0 {
		return inlineCommandResult("", InlineCommandStatusIgnored, "empty")
	}

	cmd := tokens[0]

	switch cmd {
	// === Matchers Management ===
	case "ADD", "add":
		// Parse command logLine with quote handling
		args, err := splitArgsShellStyle(commandLine[len(cmd):])
		if err != nil || len(args) < 1 {
			return inlineCommandRejected(cmd, "add_matcher", "missing matcher name")
		}

		isRegex := false
		name := args[0]
		scopeName := "line"
		patterns := []string{}
		// Check if second arg is a scope flag
		if len(args) > 1 {
			switch args[1] {
			case "--words":
				scopeName = "words"
			case "--refs":
				scopeName = "refs"
			case "--ips":
				scopeName = "ips"
			case "--code":
				scopeName = "code"
			case "--line":
				scopeName = "line"
			case "--regex":
				isRegex = true
			default:
				patterns = append(patterns, args[1])
			}
		}

		// Append any remaining args as patterns, trimming at '#' comment if present
		if len(args) > 2 {
			for _, arg := range args[2:] {
				if strings.HasPrefix(arg, "#") {
					break // Stop including args at the first '#' marker
				}
				patterns = append(patterns, arg)
			}
		}

		originalName := name
		if name[0:1] == "*" {
			name = name[1:]
		}
		if name[0:1] == "+" {
			name = name[1:]
		} else if name[0:1] == "-" {
			name = name[1:]
		}

		if len(patterns) == 0 {
			if name == "Browser" || name == "Platform" || name == "Bots" {
				isRegex = true
			} else {
				patterns = []string{name}
			}
		}
		if matcherNameExists(name) && !(name == "Bots" && isRegex && len(patterns) == 0) {
			return inlineCommandRejected(cmd, "add_matcher", "duplicate matcher name")
		}
		//newM := PattyGraph.createMatcher(name, isLikelyIPPattern(name), patterns)
		var newM *Matcher
		if isRegex {
			if name == "Browser" && len(patterns) == 0 {
				newM = newRegexMatcher(name, browserRegexString)
			} else if name == "Bots" {
				toggleBotsMatcher(false)
			} else if name == "Platform" && len(patterns) == 0 {
				newM = newRegexMatcher(name, platformRegexString)
			} else {
				index := strings.Index(commandLine, "--regex ")
				newM = newRegexMatcher(name, strings.TrimSpace(commandLine[index+8:]))
			}
		} else {
			switch scopeName {
			case "words":
				newM = WordsMatcher(name, patterns)
			case "refs":
				newM = RefsMatcher(name, patterns)
			case "ips":
				newM = IpsMatcher(name, patterns)
			case "code":
				newM = CodeMatcher(name, patterns)
			default:
				newM = PattyGraph.createMatcher(name, areLikelyIPPatterns(patterns), patterns)
			}
		}
		if newM == nil {
			return inlineCommandRejected(cmd, "add_matcher", "matcher was not created")
		}
		newM.inlineCommandAction = func() string {
			return line
		}
		placement := "before_bots"
		switch originalName[0:1] {
		case "*":
			placement = "before_lines"
			PattyGraph.matchers = insertMatcherBeforeLines(PattyGraph.matchers, newM)
		case "+":
			placement = "first"
			PattyGraph.matchers = insertMatcherFirst(PattyGraph.matchers, newM)
		case "-":
			placement = "before_bots"
			PattyGraph.matchers = insertMatcherBeforeBots(PattyGraph.matchers, newM)
		default:
			if len(newM.history) > 0 {
				placement = "first"
				PattyGraph.matchers = insertMatcherFirst(PattyGraph.matchers, newM)
			} else {
				PattyGraph.matchers = insertMatcherBeforeBots(PattyGraph.matchers, newM)
			}
		}
		botsIndex = botsMatcherIndex()
		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "add_matcher")
		result.Result["matcher_name"] = name
		result.Result["placement"] = placement
		result.Result["scope"] = scopeName
		result.Result["patterns"] = patterns
		result.Result["regex"] = isRegex
		return result
	case "select", "SELECT":
		args, err := splitArgsShellStyle(commandLine[len(cmd):])
		if err != nil {
			return inlineCommandRejected(cmd, "select_interesting", "missing scope or selection")
		}
		if len(args) == 0 {
			if PattyGraph.selectedInterestingMatcher != nil {
				PattyGraph.selectedInterestingMatcher.selectedKey = ""
				PattyGraph.selectedInterestingMatcher.selectedGraphCache = ""
				PattyGraph.selectedInterestingMatcher = nil
			}
			return inlineCommandResult(cmd, InlineCommandStatusApplied, "clear_selection")
		}
		if len(args) < 2 {
			return inlineCommandRejected(cmd, "select_interesting", "missing selection key")
		}

		scope := strings.ToLower(strings.TrimLeft(args[0], "-"))
		selection := strings.TrimSpace(strings.Join(args[1:], " "))

		var target *InterestingWordMatcher
		switch scope {
		case "words":
			target = PattyGraph.wordsMatcher
		case "refs":
			target = PattyGraph.refsMatcher
		case "ips":
			target = PattyGraph.ipsMatcher
		default:
			return inlineCommandRejected(cmd, "select_interesting", "unsupported selection scope")
		}

		if target == nil {
			return inlineCommandRejected(cmd, "select_interesting", "selection target unavailable")
		}

		idx, ok := target.selectDisplayItemByKey(selection)
		if !ok {
			return inlineCommandRejected(cmd, "select_interesting", "selection not found")
		}

		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "select_interesting")
		result.Result["matcher"] = target.mName
		result.Result["selection"] = target.selectionKey()
		result.Result["selection_index"] = idx
		return result
	case "DEL", "del", "delete", "DELETE":
		if len(tokens) < 2 {
			return inlineCommandRejected(cmd, "delete_matcher", "missing matcher name")
		}
		fromTop := true
		name := tokens[1]
		// being nice for users doing a quick edit at the cli
		// add +Matcher ...
		// del +Matcher ....
		// they can just edit the action and leave the additional info on or off
		if name[0:1] == "*" {
			name = name[1:]
		}
		if name[0:1] == "+" {
			name = name[1:]
		} else if name[0:1] == "-" {
			name = name[1:]
			fromTop = false
		}
		removeMatcherFromTop(name, fromTop)
		botsIndex = botsMatcherIndex()
		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "delete_matcher")
		result.Result["matcher_name"] = name
		result.Result["from_top"] = fromTop
		return result

	case "mode", "MODE":
		if len(tokens) < 2 {
			return inlineCommandRejected(cmd, "set_matcher_mode", "missing matcher name")
		}
		name := tokens[1]
		args, err := splitArgsShellStyle(commandLine[len(cmd):])
		if err != nil || len(args) < 2 {
			return inlineCommandRejected(cmd, "set_matcher_mode", "missing mode")
		}
		newMode, e := strconv.Atoi(args[1])
		if e != nil {
			return inlineCommandRejected(cmd, "set_matcher_mode", "invalid mode")
		}
		setMatcherMode(name, newMode)
		// being nice for users doing a quick edit at the cli
		// add +Matcher ...
		// del +Matcher ....
		// they can just edit the action and leave the additional info on or off
		if name[0:1] == "*" {
			name = name[1:]
		}
		if name[0:1] == "+" {
			name = name[1:]
		} else if name[0:1] == "-" {
			name = name[1:]
		}
		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "set_matcher_mode")
		result.Result["matcher_name"] = name
		result.Result["mode"] = newMode
		return result

	case "COLOR", "color":
		if len(tokens) < 3 {
			log.Printf("Invalid SET_COLOR usage: %s", commandLine)
			return inlineCommandRejected(cmd, "set_matcher_color", "missing matcher name or color")
		}
		name := tokens[1]
		color := tokens[2]
		if len(color) <= 2 {
			// color cannot be a valid color name or #FFFFFF value
			// must be an attempted index.
			if newIndex, err := strconv.Atoi(color); err == nil {
				if newIndex < len(AutobotColors) {
					color = AutobotColors[newIndex]
				}
			}
		}
		// Pretty sure this is unused
		if color[:1] != "[" {
			color = "[" + color + "]"
		}
		reassignMatcherColor(name, color)
		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "set_matcher_color")
		result.Result["matcher_name"] = name
		result.Result["color"] = color
		return result

	case "fact":
		if len(tokens) < 2 {
			return inlineCommandRejected(cmd, "show_fact", "missing fact name")
		}
		args, _ := splitArgsShellStyle(commandLine[len(cmd):])
		f := args[0]
		pushFactNow(f, args[1:])
		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "show_fact")
		result.Result["fact"] = f
		return result
	case "alert", "ALERT":
		args, err := splitArgsShellStyle(commandLine[len(cmd):])
		if err != nil || len(args) < 1 {
			return inlineCommandRejected(cmd, "alert", "missing matcher name")
		}
		return invokeAlertCommand(cmd, args, line)
	case "alerts", "ALERTS":
		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "list_alerts")
		result.Result["active_alerts"] = activeAlertStates()
		result.Result["flux_depth"] = fluxDepth
		return result
	// === Live Actions (No args) ===
	case "demo", "DEMO":
		PattyGraph.demo = !PattyGraph.demo
		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "toggle_demo")
		result.Result["enabled"] = PattyGraph.demo
		return result
	case "facts.rnd", "FACTS.RND":
		doRandom = !doRandom
		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "toggle_random_facts")
		result.Result["enabled"] = doRandom
		return result
	case "ticker", "TICKER":
		PattyGraph.showTicker = !PattyGraph.showTicker
		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "toggle_ticker")
		result.Result["enabled"] = PattyGraph.showTicker
		return result
	case "history", "HISTORY":
		showMetricsPanelContents = !showMetricsPanelContents
		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "toggle_fact_history")
		result.Result["enabled"] = showMetricsPanelContents
		return result
	case "expert", "EXPERT":
		expertMode = !expertMode
		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "toggle_expert")
		result.Result["enabled"] = expertMode
		return result
	case "control", "CONTROL":
		value := "on"
		if len(tokens) >= 2 {
			args, _ := splitArgsShellStyle(commandLine[len(cmd):])
			if len(args) > 0 {
				value = args[0]
			}
		}
		SetFlagByName("control", value)
		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "set_control_file_enabled")
		result.Result["value"] = enableControlFile
		return result
	case "purge", "PURGE":
		// TODO: can take optional matcher name
		purgePeakWordCommand()
		return inlineCommandResult(cmd, InlineCommandStatusApplied, "purge")
	case "pattySplat", "pattysplat", "PATTYSPLAT":
		pattySplat()
		return inlineCommandResult(cmd, InlineCommandStatusApplied, "write_splat")
	case "popBots", "popbots", "POPBOTS":
		PattyGraph.botsMatcher.migrateBots(-1)
		return inlineCommandResult(cmd, InlineCommandStatusApplied, "pop_bots")
	case "compact":
		compactCaches()
		return inlineCommandResult(cmd, InlineCommandStatusApplied, "compact_caches")
	case "dumpConfig", "dumpconfig", "DUMPCONFIG":
		dumpConfig()
		return inlineCommandResult(cmd, InlineCommandStatusApplied, "write_config")
	default:
		// === Property Settings (single arg) ===
		if len(tokens) < 2 {
			log.Printf("Missing value for property %s", cmd)
			return inlineCommandRejected(cmd, "set_flag", "missing value")
		}
		args, _ := splitArgsShellStyle(commandLine[len(cmd):])
		value := args[0]
		if !SetFlagByName(cmd, value) {
			log.Printf("Unknown inline command: %s", commandLine)
			return inlineCommandRejected(cmd, "unknown", "unknown inline command")
		}
		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "set_flag")
		result.Result["name"] = strings.ToLower(cmd)
		result.Result["value"] = value
		return result
	}
}

func invokeAlertCommand(cmd string, args []string, line string) InlineCommandResult {
	matcher := findMatcherByName(args[0])
	if matcher == nil {
		return inlineCommandRejected(cmd, "alert", "matcher not found")
	}
	if len(args) == 1 {
		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "show_alert")
		for key, value := range matcher.alertState() {
			result.Result[key] = value
		}
		return result
	}

	action := strings.ToLower(args[1])
	switch action {
	case AlertDirectionAbove, AlertDirectionBelow:
		if len(args) < 3 {
			return inlineCommandRejected(cmd, "set_alert", "missing threshold")
		}
		threshold, err := strconv.Atoi(args[2])
		if err != nil {
			return inlineCommandRejected(cmd, "set_alert", "invalid threshold")
		}
		if err := validateAlertBound(matcher, action, threshold); err != nil {
			return inlineCommandRejected(cmd, "set_alert", err.Error())
		}
		if action == AlertDirectionAbove {
			matcher.AlertAbove.set(threshold, line)
		} else {
			matcher.AlertBelow.set(threshold, line)
		}
		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "set_alert")
		result.Result["matcher"] = matcher.matcherName()
		result.Result["direction"] = action
		result.Result["threshold"] = threshold
		result.Result["flux_depth"] = fluxDepth
		return result
	case "clear":
		if len(args) > 2 {
			direction := strings.ToLower(args[2])
			if direction != AlertDirectionAbove && direction != AlertDirectionBelow {
				return inlineCommandRejected(cmd, "clear_alert", "unknown alert direction")
			}
			wasActive := clearMatcherAlertBound(matcher, direction)
			result := inlineCommandResult(cmd, InlineCommandStatusApplied, "clear_alert")
			result.Result["matcher"] = matcher.matcherName()
			result.Result["direction"] = direction
			result.Result["cleared"] = true
			result.Result["was_active"] = wasActive
			return result
		}
		aboveWasActive := matcher.AlertAbove.clear()
		belowWasActive := matcher.AlertBelow.clear()
		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "clear_alert")
		result.Result["matcher"] = matcher.matcherName()
		result.Result["cleared_above"] = true
		result.Result["above_was_active"] = aboveWasActive
		result.Result["cleared_below"] = true
		result.Result["below_was_active"] = belowWasActive
		return result
	default:
		return inlineCommandRejected(cmd, "alert", "unknown alert command")
	}
}

func findMatcherByName(name string) *Matcher {
	cleanName := strings.Trim(name, "*-+")
	for _, mf := range PattyGraph.matchers {
		matcher := mf.asMatcher()
		if matcher != nil && matcher.matcherName() == cleanName {
			return matcher
		}
	}
	return nil
}

func matcherNameExists(name string) bool {
	return findMatcherByName(name) != nil
}

func validateAlertBound(matcher *Matcher, direction string, threshold int) error {
	if threshold < 0 {
		return fmt.Errorf("alert threshold cannot be negative")
	}
	if direction == AlertDirectionBelow && threshold == 0 {
		return fmt.Errorf("below 0 is impossible for matcher counts")
	}
	switch direction {
	case AlertDirectionAbove:
		if matcher.AlertBelow.Enabled && matcher.AlertBelow.Threshold > threshold {
			return fmt.Errorf("below threshold must be <= above threshold")
		}
	case AlertDirectionBelow:
		if matcher.AlertAbove.Enabled && threshold > matcher.AlertAbove.Threshold {
			return fmt.Errorf("below threshold must be <= above threshold")
		}
	default:
		return fmt.Errorf("unknown alert direction")
	}
	return nil
}

func clearMatcherAlertBound(matcher *Matcher, direction string) bool {
	if direction == AlertDirectionAbove {
		return matcher.AlertAbove.clear()
	}
	return matcher.AlertBelow.clear()
}

func activeAlertStates() []map[string]interface{} {
	active := []map[string]interface{}{}
	for _, mf := range PattyGraph.matchers {
		matcher := mf.asMatcher()
		if matcher == nil {
			continue
		}
		active = append(active, matcher.activeAlertStates()...)
	}
	return active
}

func setMatcherMode(name string, newMode int) {
	var namedMatcher *Matcher
	cleanName := strings.Trim(name, "*-+")
	for _, mf := range PattyGraph.matchers {
		m := mf.asMatcher()
		if m != nil && m.name == cleanName {
			namedMatcher = m
			break
		}
	}
	if namedMatcher == nil {
		return
	}
	namedMatcher.displayMatchMode = newMode % 3
	namedMatcher.displayMatchedCache = ""
}

func colorInUse(color string) bool {
	for _, mf := range PattyGraph.matchers {
		m := mf.asMatcher()
		if m != nil && m.color == color {
			return true
		}
	}
	return false
}

func removeMatcher(name string) {
	removeMatcherFromTop(name, true)
}

func removeMatcherFromTop(name string, fromTop bool) {
	var matcher *Matcher

	if fromTop {
		for _, m := range PattyGraph.matchers {
			if m.matcherName() == name {
				matcher = m.asMatcher()
				break
			}
		}
	} else {
		for i := len(PattyGraph.matchers) - 1; i >= 0; i-- {
			m := PattyGraph.matchers[i]
			if m.matcherName() == name {
				matcher = m.asMatcher()
				break
			}
		}
	}
	if matcher == nil ||
		matcher == PattyGraph.bytesMatcher ||
		matcher == PattyGraph.linesMatcher ||
		matcher == PattyGraph.errsMatcher {
		return
	}
	if matcher == PattyGraph.botsMatcher {
		toggleBotsMatcher(true)
		return
	}
	newMatchers := make([]MatcherFacade, 0, len(PattyGraph.matchers)) // Preallocate
	for _, old := range PattyGraph.matchers {
		if old != matcher { // Keep all except selectedMatcher
			newMatchers = append(newMatchers, old)
		}
	}
	PattyGraph.matchers = newMatchers // Replace old slice
}

func toggleBotsMatcher(newState bool) {
	PattyGraph.botsMatcher.disableAutoAdd = newState
	if PattyGraph.botsMatcher.disableAutoAdd {
		PattyGraph.botsMatcher.setColor(PattyDisabledBotsColor)
	} else {
		PattyGraph.botsMatcher.setColor(PattyBotsColor)
	}
}
func purgePeakWordCommand() {
	PattyGraph.wordsMatcher.purgePeakWords()
	PattyGraph.refsMatcher.purgePeakWords()
	PattyGraph.ipsMatcher.purgePeakWords()
}
func pattySplat() {
	PattyGraph.printToFile()
}
func dumpConfig() {
	filename := newTimestampedFilename("pattyGraph_", ".conf")
	fullPath := filename
	if PattyGraph.pattyConfig.saveDir != "" {
		fullPath = filepath.Join(PattyGraph.pattyConfig.saveDir, filename)
	}
	f, err := os.Create(fullPath)
	if err != nil {
		return
	}
	defer f.Close()
	writeConfig(f)
}

func writeConfig(w io.Writer) {
	// TODO: Could just write everything out and then wrap all lines with the preamble
	if machineDisplayName != "" {
		io.WriteString(w, fmt.Sprintf(InlinePreamble+" title '%s'\n", machineDisplayName))
	}
	if PattyGraph.pattyConfig.saveDir != "" {
		io.WriteString(w, fmt.Sprintf(InlinePreamble+" save-dir '%s'\n", PattyGraph.pattyConfig.saveDirOriginal))
	}
	if pattyPushFactor != pattyPushFactorDefault {
		io.WriteString(w, fmt.Sprintf(InlinePreamble+" push %d\n", pattyPushFactor))
	}
	if pattyGracePeriod != pattyGracePeriodDefault {
		io.WriteString(w, fmt.Sprintf(InlinePreamble+" grace %d\n", pattyGracePeriod))
	}
	if fluxDepth != DefaultFluxDepth {
		io.WriteString(w, fmt.Sprintf(InlinePreamble+" flux %d\n", fluxDepth))
	}
	if pattyScaleFactor != pattyScaleFactorDefault {
		io.WriteString(w, fmt.Sprintf(InlinePreamble+" scale %1.1f\n", pattyScaleFactor))
	}
	if PattyGraph.pattyConfig.mbToRead != DefaultMBToRead {
		io.WriteString(w, fmt.Sprintf(InlinePreamble+" read %d\n", PattyGraph.pattyConfig.mbToRead))
	}
	if expertMode {
		io.WriteString(w, InlinePreamble+" expert\n")
	}

	// Iterate through matchers and write their inline command representation
	for _, m := range PattyGraph.matchers {
		if m == nil {
			continue
		}
		cmd := m.asInlineCommand() // to be implemented per matcher
		if cmd != "" {
			io.WriteString(w, cmd+"\n")
		}
		matcher := m.asMatcher()
		if matcher != nil && matcher.displayMatchMode != 0 {
			io.WriteString(w, fmt.Sprintf(InlinePreamble+" mode %s %d\n", matcher.name, matcher.displayMatchMode))
		}
		if matcher != nil {
			for _, alertLine := range matcher.alertConfigLines() {
				io.WriteString(w, alertLine+"\n")
			}
		}
	} // Iterate through matchers and write their inline command representation
	for _, m := range PattyGraph.matchers {
		if m == nil {
			continue
		}
		matcher := m.asMatcher()
		if matcher != nil && matcher.isColorUserAssigned {
			io.WriteString(w, fmt.Sprintf(InlinePreamble+" color %s %s\n", matcher.name, matcher.color))
		}
	}
	io.WriteString(w, fmt.Sprintf("#"+InlinePreamble+" color-index %d    # Next color index (Autogenerated)\n", colorIndex))
}

func setFlux(f int) bool {
	if f < 1 || f > 10 {
		return false
	}
	if fluxDepth != f {
		fluxDepth = f
		resetAllAlertRuntimeState()
		return true
	}
	return false
}
func SetFlagByName(key string, value string) bool {
	switch key {
	case "json":
		if value == "on" {
			generateJsonSparks = true
		} else {
			generateJsonSparks = false
		}
	case "control":
		enableControlFile = parseControlEnabled(value)
		return true
	case "push":
		if newPush, er := strconv.Atoi(value); er == nil {
			pushIncr := newPush - pattyPushFactor
			if PattyGraph.pattyPushFactorIncr(pushIncr) { // UI was already doing this so re-use until replaced
				pushFactNow("settings.push", nil)
			}
		}
		return true
	case "read":
		if newRead, er := strconv.Atoi(value); er == nil {
			PattyGraph.pattyConfig.mbToRead = newRead
		}
		return true
	case "grace":
		if newGrace, er := strconv.Atoi(value); er == nil {
			if setGracePeriod(newGrace) {
				pushFactNow("settings.grace", nil)
			}
		}
		return true
	case "flux":
		if f, er := strconv.Atoi(value); er == nil {
			if setFlux(f) {
				pushFactNow("settings.flux", nil)
			}
		}
		return true
	case "scale":
		if newScale, er := strconv.ParseFloat(value, 64); er == nil {
			// Truncate to one decimal place (flooring)
			setNewScaleFactor(newScale)
			pushFactNow("settings.scale", nil)
		}
		return true
	case "title":
		machineDisplayName = value
		return true
	case "save-dir":
		PattyGraph.pattyConfig.saveDirOriginal = value
		PattyGraph.pattyConfig.saveDir = expandUser(value)
		return true
	case "color-index":
		if newIndex, er := strconv.Atoi(value); er == nil {
			if newIndex != colorIndex {
				colorIndex = newIndex // inline assignment
				pushFactNow("settings.color-index", nil)
			}
		}
		return true
	}
	return false
}

func setNewScaleFactor(originalScale float64) {
	// if you don't put your thumb on the scale, some 0.1 steps are skipped
	// 0.01 ensures things like 0.7999999 don't mess things up. Yes it happened.
	fudgedScale := originalScale + 0.01
	newScale := math.Floor(fudgedScale*10) / 10.0
	pattyScaleFactor = newScale
	if pattyScaleFactor > pattyBurstMax {
		pattyScaleFactor = pattyBurstMax
	}
	if pattyScaleFactor < pattyBurstMin {
		pattyScaleFactor = pattyBurstMin
	}
}

func setGracePeriod(newGrace int) bool {
	if newGrace < 2 {
		newGrace = 2
	}
	if newGrace > DefaultHistoryDepth {
		newGrace = DefaultHistoryDepth + 1
	}
	if newGrace != pattyGracePeriod {
		pattyGracePeriod = newGrace
		return true
	}
	return false
}
func tabGlyph() string {
	symbols := []string{"-", "/", "|", "\\", "=", "_"}
	return symbols[PattyGraph.tabViewIndexKey%len(symbols)]
}
func columnRuler() string {
	return fillRuler(PattyPrintWidth, true)
}

func fillRuler(width int, multiLine bool) string {
	var digits1, digits10 string
	for i := 1; i <= width; i++ {
		digits1 += fmt.Sprintf("%d", i%10)
		if i%10 == 0 {
			digits10 += fmt.Sprintf("%d", (i/10)%10)
		} else {
			digits10 += " "
		}
	}
	if multiLine {
		return digits10 + "\n" + digits1
	}
	return digits1
}

// Expensive but cached
func timeScaleWithHighlights() string {
	if timeScaleCache == "" {
		if expertMode {
			timeScaleCache = createTimeScaleWithHighlights(ExpertScaleWidth)
		} else {
			timeScaleCache = createTimeScaleWithHighlights(DefaultHistoryDepth)
		}
	}
	return timeScaleCache
}

var timeScaleCache string

// Expensive but cached
func createTimeScaleWithHighlights(displayWidth int) string {
	const highlight = "[white]"
	const reset = "[default]"
	var b strings.Builder
	pos := 0
	b.WriteString(reset)
	for pos <= displayWidth {
		if pos == 0 {
			if pos == PattyGraph.miniWindowIndex {
				b.WriteString(fmt.Sprintf("%s%s%s", highlight, "/60", reset))
			} else {
				b.WriteString("/60")
			}
			pos += 3
		} else if pos >= 80 {
			if pos == PattyGraph.miniWindowIndex {
				b.WriteString(fmt.Sprintf("%s%s%s", highlight, "8", reset))
			} else {
				b.WriteString("8")
			}
			pos += 1
		} else if pos == 5 || pos%10 == 0 {
			if pos == PattyGraph.miniWindowIndex {
				b.WriteString(fmt.Sprintf("%s%2d%s", highlight, pos, reset))
			} else {
				b.WriteString(fmt.Sprintf("%2d", pos))
			}
			pos += 2
		} else if pos > 10 && pos%5 == 0 {
			if pos == PattyGraph.miniWindowIndex {
				b.WriteString(fmt.Sprintf("%s%s%s", highlight, ".", reset))
			} else {
				b.WriteString(".")
			}
			pos += 1
		} else {
			b.WriteByte(' ')
			pos += 1
		}
	}
	return b.String()
}

type BuilderComplex struct {
	graphingBuilder strings.Builder
	matcherBuilder  strings.Builder
}

// Reset clears all builders
func (b *BuilderComplex) Reset() {
	b.graphingBuilder.Reset()
	b.matcherBuilder.Reset()
}

// builders used during display. Probably could be consolidated to a builder pool across more uses but it never shows
// up in profiles now like this.
var PattyGraphBuilderComplex BuilderComplex

func updateDisplay() {
	defer PattyGraphBuilderComplex.Reset()
	// Build main display text

	var progressPanel string
	// For debugging only. Throws off click detection bc 2 lines not accounted for
	//PattyGraphBuilderComplex.graphingBuilder.WriteString(columnRuler() + "\n") // debugging ruler lines for alignment w/o manual counting

	if PattyGraph.totalLines < 10000000 {
		progressPanel = fmt.Sprintf("%7d/%d/%-4s", PattyGraph.totalLines, PattyGraph.intervalsCompleted, strings.TrimSpace(formatBytes64(PattyGraph.totalBytes)))
	} else {
		progressPanel = fmt.Sprintf("%7s/%d/%-4s", formatCountsUint64(PattyGraph.totalLines), PattyGraph.intervalsCompleted, strings.TrimSpace(formatBytes64(PattyGraph.totalBytes)))
	}

	selectionValue := "-"
	if PattyGraph.selectedGraphValue > 0 {
		selectionValue = PattyGraph.selectionValue
	}

	PattyGraphBuilderComplex.graphingBuilder.WriteString(fmt.Sprintf("[white]File: %-30s%18.18s%20.20s %-4s %20s\n",
		PattyGraph.filePath,
		machineDisplayName,
		progressPanel,
		selectionValue,
		PattyGraph.logtime.Format("Jan 02,2006 15:04:05")))
	timeScaleLine := timeScaleWithHighlights()

	progressColor := "yellow"
	if displayFreezeCountdown > 0 {
		progressColor = "red"
	}

	if expertMode {
		internalState := PattyGraph.statusLine()
		//internalState = fillRuler(37, false)
		PattyGraphBuilderComplex.graphingBuilder.WriteString(fmt.Sprintf(" ["+progressColor+"]%15.15s[white]%3d%-44s[white]%37s\n",
			renderProgressBar(), currentCycle, timeScaleLine, internalState))
	} else {
		PattyGraphBuilderComplex.graphingBuilder.WriteString(fmt.Sprintf(" ["+progressColor+"]%15.15s[white]%3d%80s\n",
			renderProgressBar(), currentCycle, timeScaleLine))

	}

	// Pre-calculate global history-wide values used by matchers once here
	// TODO: This could be moved to the push?
	if PattyGraph.overallMax <= 0 {
		// These values only change after a push(), so this tries to cache them between pushes
		// push sets overallMax to -1 to signal a recomputation be done here
		PattyGraph.overallMin, _, PattyGraph.overallMax = PattyGraph.minAvgMaxHistoryAcrossMatchers()
	}

	if showMetricsPanelContents {
		PattyGraphBuilderComplex.matcherBuilder.WriteString(metricPanelContents())
	} else {
		for _, matcherFacade := range PattyGraph.matchers {
			matcher := matcherFacade.asMatcher()
			if matcher != nil {
				PattyGraphBuilderComplex.matcherBuilder.WriteString(matcher.displayMatched())
			}
		}
	}
	var wordsPanel, refsPanel, ipsPanel string
	if displayFreezeCountdown == 0 {
		// panel 2
		wordsPanel = PattyGraph.wordsMatcher.displayMatched()
		// panel 3
		refsPanel = PattyGraph.refsMatcher.displayMatched()
		// panel 4
		ipsPanel = PattyGraph.ipsMatcher.displayMatched()
	}

	// IMPORTANT: This must be done after word matcher matching so spoofed selection values are created
	// Draws matcher labeling and sparkline
	for _, matcher := range PattyGraph.matchers {
		PattyGraphBuilderComplex.graphingBuilder.WriteString(matcher.displayString())
	}
	if PattyGraph.showTicker {
		PattyGraphBuilderComplex.graphingBuilder.WriteString("[default:-]")
		PattyGraphBuilderComplex.graphingBuilder.WriteString(currentTickerText())
		PattyGraphBuilderComplex.graphingBuilder.WriteString("[-:-]\n")
	}

	if PattyGraph.selectedInterestingMatcher != nil {
		PattyGraphBuilderComplex.graphingBuilder.WriteString(PattyGraph.selectedInterestingMatcher.displayLogLine())
	}
	if PattyGraph.selectedMatcher != nil {
		PattyGraphBuilderComplex.graphingBuilder.WriteString(PattyGraph.selectedMatcher.displayLogLine())
	}

	PattyGraph.sparklineHistoryView.SetText(PattyGraphBuilderComplex.graphingBuilder.String())
	PattyGraph.botMatchesView.SetText(PattyGraphBuilderComplex.matcherBuilder.String())
	if displayFreezeCountdown == 0 {
		PattyGraph.wordMatchesView.SetText(wordsPanel)
		PattyGraph.refsView.SetText(refsPanel)
		PattyGraph.ipsView.SetText(ipsPanel)
	}
	relayoutUI()
}

func (m *Monitor) pattyPushFactorIncr(increment int) bool {
	prePush := pattyPushFactor
	pattyPushFactor += increment
	if pattyPushFactor < 0 {
		pattyPushFactor = 0
	} else if pattyPushFactor > 11 {
		pattyPushFactor = 11
	}
	if prePush != pattyPushFactor {
		wordsPurgeInterval, refsPurgeInterval, ipsPurgeInterval := lookupPurgeIntervals(pattyPushFactor)
		m.wordsMatcher.setPurgeInterval(wordsPurgeInterval)
		m.refsMatcher.setPurgeInterval(refsPurgeInterval)
		m.ipsMatcher.setPurgeInterval(ipsPurgeInterval)
		return true
	}
	return false
}

/*
*

		PattyGraph intentionally treats the terminal screen as a rendered coordinate
		surface rather than a tree of tview widgets. The display panes are text views,
		but the interaction model is visual: clicks are interpreted by X/Y position
		against stable screen regions such as the matcher graph, matcher breakdowns,
		words, refs, and IPs.

		setUIHook is therefore the TUI controller. It maps keyboard and mouse gestures
		onto PattyGraph’s live operational model: matcher selection, interesting-key
		selection, graph value inspection, display-mode cycling, matcher promotion, and
		runtime tuning.

		Basically:
	    this is not accidental spaghetti
	    it is a deliberate controller layer
	    tview widgets are rendering surfaces, not the interaction model
	    the UI behavior is spatial and section-based
	    the function is large because the controller owns the full gesture vocabulary
*/
func setUIHook() {
	PattyGraph.app.SetMouseCapture(func(event *tcell.EventMouse, action tview.MouseAction) (*tcell.EventMouse, tview.MouseAction) {
		// Set up event handling
		mLen := len(PattyGraph.matchers)
		// TODO Probably all of this needs to be made more sensible. Let listy-things declare their extents, simulate
		// hit detection, etc. Right now, hardcode it. The UI layout is stable.
		if action == tview.MouseLeftClick {
			sHeight := PattyGraph.sparkPanelHeight()
			hasControlKey := event.Modifiers()&tcell.ModCtrl != 0
			x, y := event.Position()
			//PattyGraph.selectedGraphPosition = fmt.Sprintf("%d:%d", x, y)
			// spark graph click
			offset, _ := PattyGraph.sparklineHistoryView.GetScrollOffset()
			if x > PattyPrintWidth || y < (2-offset) {
				return event, action
			}
			// Autobot Matchers & Sparkline
			// TODO this just happens to work bc there's 3 word matchers + 1 Selection spot in the display list
			if y < sHeight {
				index := y - 2
				matcherCount := mLen - 3
				index += offset
				// matcher & ticker selection management
				if index < matcherCount {
					newM := PattyGraph.matchers[index].asMatcher()
					if PattyGraph.selectedMatcher == newM && x < 20 {
						if hasControlKey {
							//newM.displayMatchZero = !newM.displayMatchZero
							newM.displayMatchMode = (newM.displayMatchMode + 1) % 3
							newM.displayMatchedCache = ""
						} else {
							PattyGraph.setSelectedMatcher(nil)
						}
					} else {
						PattyGraph.setSelectedMatcher(newM)
						if hasControlKey {
							//newM.displayMatchZero = !newM.displayMatchZero
							newM.displayMatchMode = (newM.displayMatchMode + 1) % 3
							newM.displayMatchedCache = ""
						}
					}
				} else if PattyGraph.showTicker &&
					((PattyGraph.selectedInterestingMatcher != nil && index == matcherCount+1) ||
						(PattyGraph.selectedInterestingMatcher == nil && index == matcherCount)) {
					togglePreamble()
				}

				//// sparkgraph value selection
				//// TODO: This should probably be using index
				if y == mLen-1 && PattyGraph.selectedInterestingMatcher != nil {
					//m.setSelectedMatcher(nil)
					if x >= 20 && x < PattyPrintWidth {
						idx := x - 20
						PattyGraph.selectedGraphValue = PattyGraph.selectedInterestingMatcher.selectedHistoryAt(idx)
						PattyGraph.selectionValue = fmt.Sprintf("%d", PattyGraph.selectedGraphValue)
						if PattyGraph.showTicker {
							pushPrintStatCycles(
								PattyGraph.selectedInterestingMatcher.selectedKey,
								PattyGraph.selectedGraphValue,
								idx,
							)
						}
					}
				} else if y <= mLen-1 && PattyGraph.selectedMatcher != nil && x >= 20 && x < PattyPrintWidth {
					// Getting the idx into the spoofed wordStats
					idx := x - 19
					if PattyGraph.selectedMatcher.history != nil && idx <= len(PattyGraph.selectedMatcher.history) {
						PattyGraph.selectedGraphValue = PattyGraph.selectedMatcher.history[len(PattyGraph.selectedMatcher.history)-idx]
						if PattyGraph.selectedMatcher == PattyGraph.bytesMatcher {
							PattyGraph.selectionValue = fmt.Sprintf("%s", formatBytes(PattyGraph.selectedGraphValue))
						} else if PattyGraph.selectedMatcher == PattyGraph.linesMatcher {
							PattyGraph.selectionValue = fmt.Sprintf("%s", formatCounts(PattyGraph.selectedGraphValue))
						} else {
							if PattyGraph.selectedGraphValue > 10000 {
								PattyGraph.selectionValue = trimmedCounts(PattyGraph.selectedGraphValue)
							} else {
								PattyGraph.selectionValue = fmt.Sprintf("%d", PattyGraph.selectedGraphValue)
							}
						}
						if PattyGraph.showTicker {
							pushPrintStatCycles(
								PattyGraph.selectedMatcher.matcherName(),
								PattyGraph.selectionValue,
								idx,
							)
						}
					} else {
						// out of bounds catchall
						PattyGraph.selectedGraphValue = -1
						//pushPrintNow("bounds checker")
					}
				}
				return nil, action
				//} else if  {
				//
			} else {
				// below the spark graph. One of the four columns should take "focus" and translate the click to a
				// selection. Interesting columns only look for/print the selectionKey if they have focus
				if x <= botsDisplayWidth {
					if !showMetricsPanelContents {
						// MATCHERS Column
						offset, _ := PattyGraph.botMatchesView.GetScrollOffset()
						index := y - PattyGraph.sparkPanelHeight() + offset
						slide := 0 // simulated indexing to make it "chunkier"  (i.e. one matcher can == multiple lines)
						for i, facade := range PattyGraph.matchers {
							matcher := facade.asMatcher()
							if matcher != nil {
								//if matcher.name != "bytes" {
								//if matcher.name != "lines" && matcher.name != "bytes" {
								if index >= i+slide && index <= i+slide+matcher.matchedDisplayCount {
									if PattyGraph.selectedMatcher == matcher {
										if hasControlKey {
											//matcher.displayMatchZero = !matcher.displayMatchZero
											matcher.displayMatchMode = (matcher.displayMatchMode + 1) % 3
											matcher.displayMatchedCache = ""
										} else {
											PattyGraph.setSelectedMatcher(nil)
										}
									} else {
										PattyGraph.setSelectedMatcher(matcher)
										if hasControlKey {
											//matcher.displayMatchZero = !matcher.displayMatchZero
											matcher.displayMatchMode = (matcher.displayMatchMode + 1) % 3
											matcher.displayMatchedCache = ""
										}
									}
									break
								}
								//autoMatcher.toggleSelection(index >= i+slide && index <= i+slide+autoMatcher.matchedDisplayCount)
								slide += matcher.matchedDisplayCount
								//} else {
								//	// lines and bytes get skipped and have to reset the sliding index
								//	//autoMatcher.toggleSelection(false)
								//	slide--
								//}
							}
						}
					}
				} else if x < botsDisplayWidth+PattyGraph.wordsMatcher.displayWidth {
					// words
					offset, _ := PattyGraph.wordMatchesView.GetScrollOffset()
					index := y - PattyGraph.sparkPanelHeight() + offset - 1
					//m.setSelectedMatcher(nil)
					PattyGraph.wordsMatcher.selectDisplayItem(index)
				} else if x < botsDisplayWidth+PattyGraph.wordsMatcher.displayWidth+PattyGraph.refsMatcher.displayWidth {
					// refs
					offset, _ := PattyGraph.refsView.GetScrollOffset()
					index := y - PattyGraph.sparkPanelHeight() + offset - 1
					//m.setSelectedMatcher(nil)
					PattyGraph.refsMatcher.selectDisplayItem(index)
				} else if x < botsDisplayWidth+PattyGraph.wordsMatcher.displayWidth+PattyGraph.refsMatcher.displayWidth+PattyGraph.ipsMatcher.displayWidth {
					// ips
					offset, _ := PattyGraph.ipsView.GetScrollOffset()
					index := y - PattyGraph.sparkPanelHeight() + offset - 1
					//m.setSelectedMatcher(nil)
					PattyGraph.ipsMatcher.selectDisplayItem(index)
				}
			}

		}
		return event, action
	})

	PattyGraph.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// key input dispatch logic
		switch event.Key() {
		case tcell.KeyRune:
			//if event.Modifiers()&tcell.ModCtrl != 0 {
			switch event.Rune() {
			case '<': // closest to Ctrl-<
				setGracePeriod(pattyGracePeriod - 1)
				return nil
			case '>': // closest to Ctrl->
				setGracePeriod(pattyGracePeriod + 1)
				return nil
			case '{':
				setFlux(fluxDepth - 1)
				return nil
			case '}':
				setFlux(fluxDepth + 1)
				return nil
			case '[':
				PattyGraph.miniWindowIndex -= 5
				if PattyGraph.miniWindowIndex < 0 {
					PattyGraph.miniWindowIndex = 0
				}
				timeScaleCache = ""
				return nil
			case ']':
				PattyGraph.miniWindowIndex += 5
				if PattyGraph.miniWindowIndex > 75 {
					PattyGraph.miniWindowIndex = 75
				}
				timeScaleCache = ""
				return nil
			case 'x':
				expertMode = !expertMode
				timeScaleCache = ""
				return nil
			case 'X':
				PattyGraph.showTicker = !PattyGraph.showTicker
				timeScaleCache = ""
				return nil
			case 'f':
				// 5+1 bc current cycle
				displayFreezeCountdown = 6
				return nil
			case 'q':
				PattyGraph.app.Stop()
				return nil
			case 'F':
				// 10+1 bc current cycle
				displayFreezeCountdown = 11
				return nil
			case 'U':
				if PattyGraph.selectedMatcher == nil ||
					PattyGraph.selectedMatcher.name == "lines" ||
					PattyGraph.selectedMatcher.name == "bytes" ||
					PattyGraph.selectedMatcher.name == "errs" {
					return nil
				}

				for i := 1; i < len(PattyGraph.matchers); i++ {
					if PattyGraph.matchers[i].asMatcher() == PattyGraph.selectedMatcher {
						// Swap with the one above it
						PattyGraph.matchers[i], PattyGraph.matchers[i-1] = PattyGraph.matchers[i-1], PattyGraph.matchers[i]
						break
					}
				}
				UpdateHistoricFlags()
				return nil
			case 'D':
				if PattyGraph.selectedMatcher == nil ||
					PattyGraph.selectedMatcher.name == "lines" ||
					PattyGraph.selectedMatcher.name == "bytes" ||
					PattyGraph.selectedMatcher.name == "errs" {
					return nil
				}
				for i := 0; i < len(PattyGraph.matchers)-1; i++ {
					if PattyGraph.matchers[i+1].asMatcher() == PattyGraph.linesMatcher {
						break
					}
					if PattyGraph.matchers[i].asMatcher() == PattyGraph.selectedMatcher {
						// Swap with the one after it
						PattyGraph.matchers[i], PattyGraph.matchers[i+1] = PattyGraph.matchers[i+1], PattyGraph.matchers[i]
						break
					}
				}
				UpdateHistoricFlags()
				return nil
			}

		case tcell.KeyTab:
			if PattyGraph.demo {
				PattyGraph.demo = false
			} else {
				PattyGraph.tabViewIndexKey = (PattyGraph.tabViewIndexKey + 1) % SecondaryInfoTabDepth
			}
			return nil
		case tcell.KeyRight:
			if event.Modifiers()&tcell.ModCtrl != 0 {
				PattyGraph.pattyPushFactorIncr(1)
				return nil
			}
		case tcell.KeyLeft:
			if event.Modifiers()&tcell.ModCtrl != 0 {
				PattyGraph.pattyPushFactorIncr(-1)
				return nil
			}
		case tcell.KeyUp:
			if event.Modifiers()&tcell.ModCtrl != 0 {
				setNewScaleFactor(pattyScaleFactor + pattyBurstStep)
				return nil
			}
		case tcell.KeyDown:
			if event.Modifiers()&tcell.ModCtrl != 0 {
				setNewScaleFactor(pattyScaleFactor - pattyBurstStep)
				return nil
			}
		case tcell.KeyCtrlF:
			doRandom = !doRandom
			//firstColorWins = !firstColorWins
			return nil
		case tcell.KeyCtrlP:
			// Purge! Clear all PeakWords!
			PattyGraph.purgeAllPeakContent()
			return nil
		case tcell.KeyCtrlK:
			// Purge! Clear all PeakWords!
			generateJsonSparks = !generateJsonSparks
			return nil
		case tcell.KeyCtrlD:
			if PattyGraph.selectedMatcher == PattyGraph.botsMatcher {
				toggleBotsMatcher(!PattyGraph.botsMatcher.disableAutoAdd)
			} else {
				// Delete Selected AutoMatcher (if there is one)
				PattyGraph.deleteSelectedMatcher()
			}
			return nil
		case tcell.KeyEscape:
			if PattyGraph.selectedMatcher != nil {
				PattyGraph.setSelectedMatcher(nil)
			} else if PattyGraph.selectedInterestingMatcher != nil {
				PattyGraph.selectedInterestingMatcher.selectedKey = ""
				PattyGraph.selectedInterestingMatcher.selectedGraphCache = ""
				PattyGraph.selectedInterestingMatcher = nil
			} else {
				PattyGraph.selectedGraphValue = 0
			}
			return nil
		case tcell.KeyCtrlG:
			dumpConfig()
			return nil
		case tcell.KeyCtrlS:
			pattySplat()
			return nil
		case tcell.KeyCtrlM, tcell.KeyCtrlB, tcell.KeyCtrlN:
			if event.Key() == tcell.KeyCtrlM && PattyGraph.selectedMatcher == PattyGraph.botsMatcher {
				PattyGraph.botsMatcher.migrateBots(-1)
			} else if PattyGraph.selectedInterestingMatcher != nil {
				newPattern := PattyGraph.selectedInterestingMatcher.selectedKey
				if newPattern != "" {
					if matcherNameExists(newPattern) {
						return nil
					}
					var newM *Matcher
					fmtString := ""
					if event.Key() == tcell.KeyCtrlN {
						fmtString = "*"
					}

					switch PattyGraph.selectedInterestingMatcher {
					case PattyGraph.refsMatcher:
						newM = RefsMatcher(newPattern, []string{newPattern})
						newM.inlineCommandAction = func() string {
							return fmt.Sprintf(InlinePreamble+" add %s%s --refs", fmtString, newPattern)
						}
					case PattyGraph.ipsMatcher:
						adjusted := newPattern
						if strings.Count(newPattern, ".") == 1 {
							adjusted = newPattern + "."
						}
						newM = IpsMatcher(newPattern, []string{adjusted})
						newM.inlineCommandAction = func() string {
							return fmt.Sprintf(InlinePreamble+" add %s%s --ips", fmtString, adjusted)
						}
					case PattyGraph.wordsMatcher:
						newM = WordsMatcher(newPattern, []string{newPattern})
						newM.inlineCommandAction = func() string {
							return fmt.Sprintf(InlinePreamble+" add %s%s --words", fmtString, newPattern)
						}
					}
					if entry, exists := PattyGraph.selectedInterestingMatcher.wordFrequency[newPattern]; exists {
						newM.intervalCount = entry.count
						newM.history = entry.historySlice() // matcher creation
					} else if PattyGraph.selectedInterestingMatcher == PattyGraph.ipsMatcher && PattyGraph.ipsMatcher.ipScratch != nil {
						// This is when an item printed from displayIpGroups was selected.
						// There's probably a faux stats somewhere with this data I could have grabbed
						// but this pathway should always work too and is the originating source
						if newHistory, exists2 := PattyGraph.ipsMatcher.ipScratch.prefixHistorAggregateBufs[newPattern]; exists2 {
							newM.intervalCount = PattyGraph.ipsMatcher.ipScratch.prefixCounts[newPattern]
							newM.history = newHistory.Slice()
						}
					}

					if event.Key() == tcell.KeyCtrlB {
						PattyGraph.matchers = insertMatcherBeforeBots(PattyGraph.matchers, newM)
					} else if event.Key() == tcell.KeyCtrlN {
						PattyGraph.matchers = insertMatcherBeforeLines(PattyGraph.matchers, newM)
					} else {
						PattyGraph.matchers = insertMatcherFirst(PattyGraph.matchers, newM)
					}
				}
			}
			return nil
		}
		return event // Pass other events through
	})
}

func (m *Monitor) setSelectedMatcher(matcher *Matcher) {
	if m.selectedMatcher != nil {
		m.selectedMatcher.displayMatchedCache = ""
	}
	if matcher != nil {
		matcher.displayMatchedCache = ""
		if m.showTicker {
			matcher.pushStats()
		}
	}
	m.selectedMatcher = matcher // only mutator
}
func (m *Monitor) createMatcher(newPattern string, startsWith bool, patterns []string) *Matcher {
	var newM *Matcher
	if startsWith {
		// An IP matcher whether from config, inline or gesture
		//newM = StartsWithMatcher(newPattern, patterns)
		newM = IpsMatcher(newPattern, patterns)
	} else {
		newM = SimplePredicateMatcher(newPattern, patterns)
	}
	//var existingMatcher *InterestingWordMatcher
	for _, wordMatcher := range []*InterestingWordMatcher{m.ipsMatcher, m.refsMatcher, m.wordsMatcher} {
		if entry, exists := wordMatcher.wordFrequency[newPattern]; exists {
			newM.intervalCount = entry.count
			newM.history = entry.historySlice() // matcher creation
			//newHistory := make([]int, entry.historyLength())
			//copy(newHistory, entry.historySlice()) // gets reversed
			//newM.history = reversedCopy(newHistory)
			//existingMatcher = wordMatcher
			break
		}
	}
	// might need to be a part of actually adding?
	//if existingMatcher != nil && existingMatcher.wordFrequency[newPattern] != nil {
	//	existingMatcher.wordFrequency[newPattern].spawned = true
	//}
	return newM
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

var startTime time.Time

// Starts the UI and refreshes it every second to display the latest logLine intervalCount
func startUI() {
	// force the first relayout
	layoutUI()
	setUIHook()
	// Yes, here again on purpose
	initCycle()
	app := PattyGraph.app
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		startTime = time.Now()
		defer ticker.Stop()
		for range ticker.C {
			app.QueueUpdateDraw(func() {
				mu.Lock()
				defer mu.Unlock()
				incrementCycle()
				if currentCycle%displayMod == 0 {
					updateDisplay()
				}
				if currentCycle >= DefaultIntervalSize {
					var sidecarSnapshot SidecarInterval
					if generateSidecarJSONL {
						// Sidecar interval event: capture the just-completed interval before
						// push() rolls live counters into history and resets interval state.
						// The existing TUI cycle remains the timing authority for now.
						sidecarSnapshot = PattyGraph.SidecarSnapshotBeforePush()
					}
					push()
					if generateSidecarJSONL {
						if err := PattyGraph.WriteSidecarJSONL(sidecarSnapshot, ""); err != nil {
							log.Printf("PattyLog jsonl write failed: %v", err)
						}
					}
					PattyGraph.writePendingAlertTransitionsJSONL()
					PattyGraph.clearPendingAlertTransitions()
					resetCycle()
				}
			})
		}
	}()
	if err := PattyGraph.app.Run(); err != nil {
		log.Fatalf("Failed to start tview application: %v\n", err)
	}
}

func relayoutUI() {
	// This could try to be smarter but its supposed to be a lightweight call
	newHeight := PattyGraph.sparkPanelHeight()
	PattyGraph.layout.ResizeItem(PattyGraph.sparklineHistoryView, newHeight, 1)
}

func layoutUI() {
	PattyGraph.sparklineHistoryView.SetDynamicColors(true).SetScrollable(true).SetTextAlign(tview.AlignLeft)
	// Main display layout with top view and bottom three views
	PattyGraph.layout = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(PattyGraph.sparklineHistoryView, PattyGraph.sparkPanelHeight(), 1, false). // Main display view at the top
		AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(PattyGraph.botMatchesView, botsDisplayWidth, 1, false). // yes, 1 wider bc
			AddItem(PattyGraph.wordMatchesView, PattyGraph.wordsMatcher.displayWidth, 1, false).
			AddItem(PattyGraph.refsView, PattyGraph.refsMatcher.displayWidth, 1, false).
			AddItem(PattyGraph.ipsView, PattyGraph.ipsMatcher.displayWidth, 1, false),
			0, 1, false) // Bottom row with 4 columns
	PattyGraph.app.SetRoot(PattyGraph.layout, true).EnableMouse(true)
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
func (m *Monitor) sparkPanelHeight() int {
	h := len(m.matchers) - 1 // plain unadjusted height
	if m.selectedMatcher != nil || m.selectedInterestingMatcher != nil {
		h += 2 // adding two lines for linesource at the end
	}
	if m.selectedInterestingMatcher != nil {
		h += 1 // adding a line for spoofed sparkgraph
	}
	if PattyGraph.showTicker {
		h += 1 // adding a line for the ticker
	}
	return h
}
func (m *Monitor) sparkPanelHeightOld() int {
	selected := -1
	if m.selectedInterestingMatcher != nil {
		matcher := m.selectedInterestingMatcher
		if matcher.selectedKey != "" {
			stats := matcher.wordFrequency[matcher.selectedKey]
			if stats != nil {
				selected = 2
			} else {
				selected = 2
			}
		}
	} else if m.selectedMatcher != nil {
		selected = 1
	}
	if PattyGraph.showTicker {
		selected++
	}
	return len(m.matchers) + selected
}

func (mon *Monitor) statusLine() string {
	status := strings.Builder{}
	lastMonitorMax := lastMonitorMaxBuf.Latest()
	lastLinesMax := lastLinesBuf.Latest()
	lastLastLinesMax := lastLinesBuf.Penultimate()
	lastBytesMax := lastBytesBuf.Latest()
	lastLastBytesMax := lastBytesBuf.Penultimate()

	status.WriteString(fmt.Sprintf("%4s", trimmedCounts(lastMonitorMax)))

	if lastLinesMax > lastLastLinesMax {
		status.WriteString(upArrow)
	} else if lastLinesMax < lastLastLinesMax {
		status.WriteString(downArrow)
	} else {
		status.WriteString(" ")
	}
	status.WriteString(fmt.Sprintf("%4s", trimmedCounts(lastLinesMax)))

	if lastBytesMax > lastLastBytesMax {
		status.WriteString(upArrow)
	} else if lastBytesMax < lastLastBytesMax {
		status.WriteString(downArrow)
	} else {
		status.WriteString(" ")
	}
	status.WriteString(fmt.Sprintf("%4s", strings.TrimSpace(formatBytes(lastBytesMax))))

	if generateJsonSparks {
		status.WriteString("@")
	} else {
		status.WriteString(" ")
	}

	status.WriteString(fmt.Sprintf("%2d", fluxDepth))

	status.WriteString(tabGlyph())
	status.WriteString(fmt.Sprintf("{%d:", pattyPushFactor))
	status.WriteString(fmt.Sprintf("%d.%d.%d",
		mon.wordsMatcher.timeToLive,
		mon.refsMatcher.timeToLive,
		mon.ipsMatcher.timeToLive,
	))
	status.WriteString("}")
	status.WriteString(fmt.Sprintf("%2d", pattyGracePeriod))
	status.WriteString(fmt.Sprintf("%0.1f", pattyScaleFactor))
	result := strings.TrimSpace(status.String())
	if len(result) > 37 {
		return result[len(result)-37:]
	}
	return result
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

type SparklineData struct {
	Name  string `json:"name"`
	X     []int  `json:"x"`
	Color string `json:"color"`
}

// strip square brackets and resolve to hex
func tcellColorToHex(raw string) string {
	name := strings.Trim(raw, "[]")
	color := tcell.GetColor(name) // works for both named and #hex values

	r, g, b := color.RGB()
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

func (m *Monitor) writeSparklineJSON() {
	// Build filename using the same pattern as pattySplat
	path := "sparkgraph.json"
	if PattyGraph.pattyConfig.saveDir != "" {
		path = filepath.Join(PattyGraph.pattyConfig.saveDir, "sparkgraph.json")
	}

	//PattyGraph.mu.RLock()
	//defer PattyGraph.mu.RUnlock()

	var payload []SparklineData
	for _, mf := range PattyGraph.matchers {
		m := mf.asMatcher()
		if m == nil {
			continue
		}
		payload = append(payload, SparklineData{
			Name:  m.matcherName(),
			X:     m.history,
			Color: tcellColorToHex(m.color),
		})
	}

	data := struct {
		Timestamp  time.Time       `json:"timestamp"`
		Sparklines []SparklineData `json:"sparklines"`
	}{
		Timestamp:  time.Now(),
		Sparklines: payload,
	}

	// Write atomically
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err == nil {
		json.NewEncoder(f).Encode(data)
		f.Close()
		os.Rename(tmp, path)
	}
}

var facts = NewFactoidGenerator()

func colorText(s string, color string) string {
	var b strings.Builder
	for _, r := range s {
		b.WriteString(fmt.Sprintf("[%s]%c", color, r))
	}
	return b.String()
}
func gradientText(s string, colors []string) string {
	var b strings.Builder
	for i, r := range s {
		color := colors[i%len(colors)]
		b.WriteString(fmt.Sprintf("[%s]%c", color, r))
	}
	return b.String()
}

var (
	tickerBuffer        string
	tickerVisibleOffset int // scroll position, in visible characters
)
