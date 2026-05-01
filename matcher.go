// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"fmt"
	"log"
	"math"
	"regexp"
	"sort"
	"strings"
)

/*
 * Matcher
 *
 * The whole point of pattyGraph is to hold an ordered set of matcher and wordMatchers while the main loop
 * drops individual log lines through. All significant work within pattyGraph is going to be done by a matcher
 * or a time based wordMatcher. These are the matchers. Everything in the graph and down the left hand side
 * of the display are all matchers.
 *
 * There's three types of these matchers now in use: Simple, Bots/Regex, lines/bytes
 *
 *	Simple - These are the auto-inserted autobot detectors like Googlebot, bingbot, etc. They search for
 *	their string in user-agent and they capture the ip's that matched and how many times. These are the name:text_pattern
 *	entries read from file
 *
 *	Regex - Bots is the only live version. This was the original matcher that matches based on a regex, capture
 *	what bot matched. Further, Bots will spawn the top matcher if it itself is the top matcher at the end of an interval.
 *	There's no reason more matchers could be made like this (restricted url matcher?) but Bots is the one. The browser
 *	identification matcher 'Brows' was exactly like Bots, just not "anointed" as being special enough to spawn autobots.
 *	Bots' special treatment could be applied more widely if the situation needed it.
 *
 *	lines/bytes - These don't have patterns, are technically called Simple (i.e. not regex) Use hint text and don't do any
 *	matching but are acting on their specific semantics. Lines, counts lines, bytes counts bytes but what they contribute is
 *	that they know how to play the history/sparkline game.
 *
 * A rewrite would gather these into more explicit ways rather than the spread around the code is has now. isAddedAutobot() and
 * name == "Bots" and the equivalents
 *
 * There's also "historical" matchers, these are the autobots above Bots and Bots. They all compete for sparline global
 * graph scaling. lines/bytes and things like brows (after Bots in the list) do not
 *
 * See Monitor for matcher layering
 */

type Matcher struct {
	name    string
	history []int // To store the history of counts
	color   string
	// todo: invert this logic later when other changes are known to be stable
	// Bots, lines, bytes, errs -- meant to also distinguish regex and non-regex but now there's no real regex
	// all non-system matchers are 'true' while Bots, lines, bytes, errs are 'false'
	isAddedAutobot    bool
	useRegexMatchKeys bool
	predicateFuncs    []func() (bool, [][]string)

	isColorUserAssigned bool
	intervalCount       int            // how many matches this interval
	matchCountsMap      map[string]int // word/ip -> interval match count
	ipGroupsCountsMap   map[string]int // prefix -> total match count (summed from matchCountsMap)
	ipGroupsMap         map[string]int // prefix -> number of IPs (members)
	lastIntervalCount   int
	lastCycleCount      int
	bytesServed         int

	matchedDisplayCount int // cache for how many subgroups printed for click selection later

	entryDisplayLineFunc                             func(entry matchEntry) string
	entryDisplaySort                                 func([]matchEntry)
	onMatchBehavior                                  func(prefetch [][]string)
	customizeDisplay                                 func(displayColor, stressColor string) string
	tagIpAction                                      func() bool
	inlineCommandAction                              func() string
	displayMatchMode                                 int
	firstMatchLine, lastMatchLine, intervalMatchLine string

	disableAutoAdd bool
	isHistorical   bool // if true, use global min/avg/max for graph scale (saves having to look it up)
	// Cache fields
	historySparklineCache string
	displayMatchedCache   string
	matchedBuilder        strings.Builder
}

func NewPredicateMatcher(name string) *Matcher {
	m := &Matcher{
		name:                name,
		color:               "[white]",
		history:             make([]int, 0, DefaultHistoryDepth), // Pre-allocate capacity for efficiency
		matchCountsMap:      make(map[string]int, 10),
		ipGroupsMap:         make(map[string]int, 10),
		ipGroupsCountsMap:   make(map[string]int, 10),
		inlineCommandAction: func() string { return "" },
	}
	m.entryDisplayLineFunc = m.entryDisplayLine
	m.entryDisplaySort = m.standardEntrySort
	m.onMatchBehavior = m.standardMatchedAction
	m.tagIpAction = m.defaultTagIpAction
	return m
}

func (m *Matcher) asMatcher() *Matcher {
	return m
}
func (m *Matcher) getCount() int { return m.intervalCount }

func (m *Matcher) changeRate() float64 {
	// always update this for the next time around (i.e. finally {} )
	defer func() { m.lastCycleCount = m.intervalCount }()

	if m.lastCycleCount == 0 {
		if m.intervalCount > 0 {
			return 1.5
		}
		return 1.0
	}
	// cycle reset happened, reset last memory
	if m.intervalCount < m.lastCycleCount {
		m.lastCycleCount = 0
	}
	previousRate := float64(m.lastIntervalCount) / float64(DefaultIntervalSize)
	span := m.intervalCount - m.lastCycleCount
	return float64(span) / previousRate
}

// Zero out counters for the next interval & migration measurement
func (m *Matcher) postEndInterval() {
	for bot, _ := range m.matchCountsMap {
		m.matchCountsMap[bot] = 0
	}
	for prefix, _ := range m.ipGroupsCountsMap {
		m.ipGroupsCountsMap[prefix] = 0
	}
}

var botsMigrated = 0

// TODO: this is some of the oldest remaining code. Lots of it doesn't make sense any longer.
func (m *Matcher) migrateBots(threshold float64) {
	var topMatcher MatcherFacade
	botsIndex = botsMatcherIndex()
	maxSiblingCount := 0
	if threshold >= 0 {
		// Iterate over all matchers to find the one with the highest intervalCount
		for i, matcher := range PattyGraph.matchers {
			// Assuming Matcher has a field `intervalCount` which we can access directly
			if i < botsIndex && matcher.getCount() > maxSiblingCount {
				maxSiblingCount = matcher.getCount()
				topMatcher = matcher
			}
		}
		// Only migrate a bot if we're in the top spot
		if topMatcher != m {
			return
		}
	}
	topCount := 0
	var topBot string
	for bot, count := range m.matchCountsMap {
		if topCount < count {
			topCount = count
			topBot = bot
		}
	}
	if topCount == 0 {
		return
	}
	if topCount < m.intervalCount {
		m.intervalCount -= topCount
	} else {
		topCount = m.intervalCount
		m.intervalCount = 0
	}

	topBotMatcher := SimplePredicateMatcher(topBot, []string{topBot})
	topBotMatcher.intervalCount = topCount
	topBotMatcher.inlineCommandAction = func() string {
		return fmt.Sprintf("!!! add %s", topBot)
	}
	PattyGraph.matchers = insertMatcherBeforeBots(PattyGraph.matchers, topBotMatcher)
	botsMigrated++
	delete(m.matchCountsMap, topBot)
}

/************************************************************************
Predicate func matcher creators below here.
*/

// Matches every line. For use by lines, bytes & errs only
func NewLineMatcher(name, color string) *Matcher {
	m := NewPredicateMatcher(name)
	m.setColor(color)
	simplePredicate := func() (bool, [][]string) {
		return true, nil
	}
	m.predicateFuncs = []func() (bool, [][]string){simplePredicate}
	m.customizeDisplay = sharedSystemDisplayFunc
	m.inlineCommandAction = func() string {
		return ""
	}
	m.displayMatchMode = 1
	return m
}

func SimplePredicateMatcher(name string, patternTexts []string) *Matcher {
	m := NewPredicateMatcher(name)
	for _, prefix := range patternTexts {
		p := prefix // capture the current value to ease the func's closure semantics
		m.predicateFuncs = append(m.predicateFuncs, func() (bool, [][]string) {
			return strings.Contains(currentLine.logLine, p), nil
		})
	}
	// todo: I think this is both wrong and ignored
	m.isAddedAutobot = true
	return m
}
func RefsMatcher(name string, patternTexts []string) *Matcher {
	m := NewPredicateMatcher(name)
	// Create one predicate function for each pattern
	for _, word := range patternTexts {
		w := word // capture the current value to ease the func's closure semantics
		m.predicateFuncs = append(m.predicateFuncs, func() (bool, [][]string) {
			return strings.Contains(currentLine.referer, w), nil
		})
	}
	// todo: I think this is both wrong and ignored
	m.isAddedAutobot = true
	return m
}
func CodeMatcher(name string, patternTexts []string) *Matcher {
	m := NewPredicateMatcher(name)
	// Create one predicate function for each pattern
	for _, errPattern := range patternTexts {
		w := errPattern // capture the current value to ease the func's closure semantics
		m.predicateFuncs = append(m.predicateFuncs, func() (bool, [][]string) {
			return strings.Contains(currentLine.respCode, w), nil
		})
	}
	// todo: I think this is both wrong and ignored
	m.isAddedAutobot = true
	return m
}
func WordsMatcher(name string, patternTexts []string) *Matcher {
	m := NewPredicateMatcher(name)
	// Create one predicate function for each pattern
	for _, word := range patternTexts {
		w := word // capture the current value to ease the func's closure semantics
		m.predicateFuncs = append(m.predicateFuncs, func() (bool, [][]string) {
			return strings.Contains(currentLine.userAgent, w) || strings.Contains(currentLine.request, w), nil
		})
	}
	// todo: I think this is both wrong and ignored
	m.isAddedAutobot = true
	return m
}
func IpsMatcher(name string, patternTexts []string) *Matcher {
	m := NewPredicateMatcher(name)
	// Create one predicate function for each pattern
	for _, prefix := range patternTexts {
		dotCount := strings.Count(prefix, ".")
		if dotCount == 3 && prefix[len(prefix)-1] != '.' {
			// expected pattern is full ip xxx.xxx.xxx.xxx
			targetIp := stringInterner.Intern(prefix)
			m.predicateFuncs = append(m.predicateFuncs, func() (bool, [][]string) {
				return currentLine.ip == targetIp, nil
			})
		} else if dotCount == 2 && prefix[len(prefix)-1] == '.' {
			// expected pattern is prefix: xxx.xxx.
			asPrefix := prefixInterner.Intern(prefix)
			m.predicateFuncs = append(m.predicateFuncs, func() (bool, [][]string) {
				return currentLine.ipPrefix == asPrefix, nil
			})
		} else {
			// whatever
			asPrefix := stringInterner.Intern(prefix)
			m.predicateFuncs = append(m.predicateFuncs, func() (bool, [][]string) {
				//return line.ipPrefix == p || line.ip == p || strings.HasPrefix(line.ip, p), nil
				return strings.HasPrefix(currentLine.ip, asPrefix), nil
			})
		}
	}
	m.isAddedAutobot = true
	return m
}

func StartsWithMatcher(name string, patternTexts []string) *Matcher {
	m := NewPredicateMatcher(name)
	// Create one predicate function for each pattern
	for _, prefix := range patternTexts {
		p := prefix // capture the current value to ease the func's closure semantics
		m.predicateFuncs = append(m.predicateFuncs, func() (bool, [][]string) {
			return strings.HasPrefix(currentLine.logLine, p), nil
		})
	}
	m.isAddedAutobot = true
	return m
}

func (m *Matcher) matcherName() string {
	return m.name
}
func (m *Matcher) setColor(color string) {
	// color and counts cause the display to need to be regenerated
	m.displayMatchedCache = ""
	m.color = color
}

// All callers to create a Matcher also supply this at creation time so this should never be nil
func (m *Matcher) asInlineCommand() string {
	return m.inlineCommandAction()
}

// push appends the current intervalCount to history, resets intervalCount, and maintains a max history length of DefaultHistoryDepth
func (m *Matcher) push() {
	// TODO: I want to reverse this appending direction someday
	//       Reversing has graphing and WordStats implications
	//       WordStats did it right and reverses itself when graphing
	// Append the current intervalCount to history
	m.history = append(m.history, m.intervalCount)

	// Reset the intervalCount to 0
	m.lastIntervalCount = m.intervalCount
	m.intervalCount = 0

	// Ensure the history length does not exceed DefaultHistoryDepth
	if len(m.history) > DefaultHistoryDepth {
		// Remove the oldest entry if we exceed DefaultHistoryDepth elements
		m.history = m.history[1:]
	}
	m.displayMatchedCache = ""
	m.historySparklineCache = ""
	m.postEndInterval()
}

/*
*
This is the heart of it all and it used to be a logical maze with different behaviors embedded here.
Now it's simplified with specialized behavior created at the call site.
Where there used to be different branches for wildcard matching or logical regex holdouts, it's all
determined at the call site now. Callers can create any sort of predicate they want now and even provide
custom actions. Bots, lines, bytes & errs all use these facilities as well as created autobots.
standardMatchedAction turns into a base level, poor man's data striping of sorts.

regarding the standardMatchedAction call:

	/**
		Below is for the case where Googlebot has seen an ip that used a Googlebot user-agent
	    but now is issuing requests that no longer have the Googlebot user-agent. IP reuse so
		soon implies its the same client but now saying they're someone else.
		Its sticky and feels like a bug but in practice it seems to work out. It wasn't by
		planning to do it this way, the two different branches that evolved organically, both
		had the same lines within them after all of the logic juggling that was in this function
		before being rewritten to use passed funcs for predicates & behavior.
*/
func (m *Matcher) match() bool {
	for _, pFunc := range m.predicateFuncs {
		if passes, prefetch := pFunc(); passes {
			if m.firstMatchLine == "" {
				m.firstMatchLine = currentLine.logLine
			}
			if m.intervalMatchLine == "" {
				m.intervalMatchLine = currentLine.logLine
			}
			m.lastMatchLine = currentLine.logLine
			m.displayMatchedCache = ""
			// This is m.standardMatchedAction() unless overridden post-creation time
			m.onMatchBehavior(prefetch)
			return true
		}
	}
	// if we've seen the ip, do the standard match action but don't return true
	// (marks the request as being related but not officially matched)
	if _, exists := m.matchCountsMap[currentLine.ip]; exists {
		m.standardMatchedAction(nil)
	}
	return false
}

func (m *Matcher) displayLogLine() string {
	if PattyGraph.tabViewIndexKey == 0 {
		return m.displayFirstLine()
	} else if PattyGraph.tabViewIndexKey == 1 {
		return m.displayFirstIntervalLine()
	}
	return m.displayLastLine()
}
func (m *Matcher) displayFirstIntervalLine() string {
	if m.intervalMatchLine == "" {
		return ""
	}
	return prettyPrintLogLine(m.intervalMatchLine,
		"",
		m.color)
}
func (m *Matcher) displayFirstLine() string {
	if m.firstMatchLine == "" {
		return ""
	}
	return prettyPrintLogLine(m.firstMatchLine,
		"",
		m.color)
}

func (m *Matcher) displayLastLine() string {
	if m.lastMatchLine == "" {
		return ""
	}
	return prettyPrintLogLine(m.lastMatchLine,
		"",
		m.color)
}
func (m *Matcher) standardMatchedAction(_ [][]string) {
	if _, exists := m.matchCountsMap[currentLine.ip]; !exists {
		m.ipGroupsMap[currentLine.ipPrefix]++
	}
	m.ipGroupsCountsMap[currentLine.ipPrefix]++
	m.matchCountsMap[currentLine.ip]++
	m.intervalCount++
	m.bytesServed += currentLine.bytesValue
	if currentLine.captureColor == "" && m != PattyGraph.botsMatcher {
		currentLine.captureColor = m.color
	}
}

// These must be kept in sync to pretend its not getting hardcoded for efficiency
var BotsSearchTerms = []string{"bot", "spider", "crawler", "agent"}

// unrolled and optimized
func isBotUAFaster() (bool, [][]string) {
	userAgent := currentLine.userAgent
	l := len(userAgent)
	for i := 0; i < l; i++ {
		c := userAgent[i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}

		switch c {
		case 'b':
			if i+3 <= l &&
				(userAgent[i]|0x20) == 'b' &&
				(userAgent[i+1]|0x20) == 'o' &&
				(userAgent[i+2]|0x20) == 't' {
				return true, nil
			}
		case 's':
			if i+6 <= l &&
				strings.EqualFold(userAgent[i:i+6], "spider") {
				return true, nil
			}
		case 'c':
			if i+7 <= l &&
				strings.EqualFold(userAgent[i:i+7], "crawler") {
				return true, nil
			}
		case 'a':
			if i+5 <= l &&
				strings.EqualFold(userAgent[i:i+5], "agent") {
				return true, nil
			}
		}
	}
	return false, nil
}

// Optimized version of findBotWord
func findBotWordFast(userAgent string) (string, bool) {
	// This is the toLower user-agent token collection Nowhere else uses this,if it did,
	// it should be stored on lineSource
	tokens := fastFieldsASCIIBuf(userAgent, &botsFieldsBuf) // findBotWordFast
	for _, token := range tokens {
		tlen := len(token)
		for _, keyword := range BotsSearchTerms {
			klen := len(keyword)
			if tlen <= klen {
				continue
			}
			if strings.EqualFold(token[tlen-klen:], keyword) {
				return token, true
			}
		}
	}
	return "", false
}

func (m *Matcher) minMaxHistory() (int, int) {
	if len(m.history) == 0 {
		return 0, 0 // Return a default value if history is empty
	}

	minV, maxV := m.history[0], m.history[0]
	for _, value := range m.history {
		if value > maxV {
			maxV = value
		}
		if value < minV {
			minV = value
		}
	}
	return minV, maxV
}

// generateSparkline generates a sparkline for an array of integers
func (m *Matcher) generateSparkline(bottom int, top int) string {
	// caller must have called appropriate locks

	history := m.history
	if len(history) == 0 {
		return ""
	}
	maxVal := top
	scaledBottom := bottom

	if !m.isHistorical {
		// "lines" and "bytes"
		scaledBottom, maxVal = m.minMaxHistory()
		scaledBottom = scaledBottom * 9 / 10
	}
	return sparklineFromArray(scaledBottom, maxVal, history)
}

// So much is historical and cobbled together. Printing/formatting really needs to be brought together better
// displayString generates a display string for the matcher showing the current intervalCount and sparkline
func (m *Matcher) displayString() string {
	//globalBottom, globalTop := PattyGraph.overallMin, PattyGraph.overallMax
	// Convert the current intervalCount to a string
	//currentCount := strconv.Itoa(matcher.intervalCount)
	var currentCount string
	// add units for bytes
	if m.matcherName() == "bytes" {
		currentCount = formatBytes(m.intervalCount)
	} else {
		currentCount = formatCounts(m.intervalCount)
	}

	// Get the most recent historical value if available, or default to "0"
	recentValue := "0"
	if len(m.history) > 0 {
		// add units for bytes
		if m.matcherName() == "bytes" {
			recentValue = formatBytes(m.previousValue())
		} else {
			recentValue = formatCounts(m.previousValue())
		}
	}

	// Generate the sparkline based on recent history
	if m.historySparklineCache == "" {
		m.historySparklineCache = m.generateSparkline(PattyGraph.overallMin, PattyGraph.overallMax)
	}

	currentArrow := levelArrow
	stressColor := m.color
	if m.isHistorical {
		stressColor = PattyBotsColor
	}
	// Needs to be at least one count per cycle expected (i.e. was last interval > 60)
	if m.lastIntervalCount >= DefaultIntervalSize {
		rate := m.changeRate()
		if rate >= 1.1 {
			stressColor = "[green]"
			currentArrow = upArrow
		} else if rate <= 0.9 {
			stressColor = "[red]"
			currentArrow = downArrow
		} else {
			currentArrow = levelArrow
		}
	}

	divider := "|"
	if m.isHistorical && m.previousValue() > 0 && m.previousValue() >= lastMonitorMaxBuf.Latest() {
		divider = upArrow
	}
	if m.isHistorical {
		divider = PattyBotsColor + divider + m.color
	}
	finalFmt := ""
	if m.customizeDisplay != nil {
		dc := m.displayColor()
		// lines, bytes & errs swap some of the color stress ordering so they get a customized call here for formatting
		finalFmt = m.customizeDisplay(dc, stressColor)
	} else {
		finalFmt = m.displayColor() + "%-10.10s%4s" + stressColor + "%s" + m.displayColor() + "%4s%1s%s[-:-]\n"
	}
	return fmt.Sprintf(finalFmt, m.name, currentCount, currentArrow, recentValue, divider, m.historySparklineCache)
}

func (m *Matcher) displayColor() string {
	if PattyGraph.selectedMatcher == m {
		return fmt.Sprintf("[%s:%s]", m.color[1:len(m.color)-1], PattyHighlight)
	}
	return m.color
}

func (m *Matcher) expandGlyph() string {
	switch m.displayMatchMode {
	case 0: // Should be skipped
		return " "
	case 1:
		return "·"
	case 2:
		return ":"
	default: // should never happen
		return "!" // can't happen
	}
}

// displayMatched returns a formatted string of all unique matched entries and their counts,
// sorted by intervalCount in descending order and alphabetically by match for ties.
func (m *Matcher) displayMatched() string {
	defer m.matchedBuilder.Reset()

	if m.displayMatchedCache != "" {
		return m.displayMatchedCache
	}
	m.matchedDisplayCount = 0
	// Collect entries into a slice for sorting
	var entries []matchEntry
	entrySum := 0
	for match, count := range m.matchCountsMap {
		entrySum += count
		if count == 0 && m.displayMatchMode > 1 || count > 0 {
			entries = append(entries, matchEntry{match, count})
		}
	}

	// Sort entries by intervalCount in descending order, with a secondary alphabetical sort on match
	m.entryDisplaySort(entries)

	// Build the result string
	result := m.matchedBuilder

	// This used to be for coexistence with tview color directives
	if m.isAddedAutobot || !m.useRegexMatchKeys {
		// this implies this was autoadded like GoogleBot and a capture group was injected and now we're
		// trying to give some group ip info instead of a list of 500 ips
		// Singleton display
		countText := fmt.Sprintf("%s(%d)", m.name, len(m.matchCountsMap))
		servedText := fmt.Sprintf("%s", formatCounts(entrySum))

		if m.displayMatchMode == 0 {
			result.WriteString(fmt.Sprintf(m.displayColor()+"%-17.17s%4s%3s[-:-]\n", countText,
				strings.TrimSpace(formatBytes(m.bytesServed)), servedText))
		} else {
			result.WriteString(fmt.Sprintf(m.displayColor()+"%-16.16s%1s%4s%3s[-:-]\n", countText, m.expandGlyph(),
				strings.TrimSpace(formatBytes(m.bytesServed)), servedText))
		}
		var counts, groups map[string]int
		// Index display mode (mode == 0) will have these be empty and drop through
		// to the results caching at the end.
		if m.displayMatchMode > 0 {
			//groups, counts = CountIPsByPrefix(m.matchCountsMap)
			groups = m.ipGroupsMap
			counts = m.ipGroupsCountsMap
		}
		// Create a slice of groups for sorting
		type prefixCount struct {
			prefix string
			count  int
		}
		var sortedGroups []prefixCount
		for prefix, _ := range groups {
			count := counts[prefix]
			if count == 0 && m.displayMatchMode > 1 || count > 0 {
				sortedGroups = append(sortedGroups, prefixCount{prefix, count})
			}
		}
		// Sort the slice by intervalCount in descending order
		sort.Slice(sortedGroups, func(i, j int) bool {
			if sortedGroups[i].count == sortedGroups[j].count {
				return sortedGroups[i].prefix < sortedGroups[j].prefix
			}
			return sortedGroups[i].count > sortedGroups[j].count
		})
		// TODO 10 should be configurable
		const matcherGroupingFactor = 10
		// Print the sorted results
		for _, group := range sortedGroups {
			if groups[group.prefix] >= matcherGroupingFactor { // Only print groups with large enough intervalCount
				groupString := fmt.Sprintf("%s*.*(%d)", group.prefix, groups[group.prefix])
				result.WriteString(fmt.Sprintf(m.displayColor()+" %-16s %5s  [-:-]\n", groupString, formatCounts(group.count)))
				m.matchedDisplayCount++
			}
		}
	} else {
		// TODO: make this be a func call instead of an else branch that other future matchers could use as well
		// Multi-match display for Bots & errs only
		// isAddedAutobot is always false
		// "Bots" & "errs" and maybe future custom matchers
		linesFudgeFactor := 0
		if m == PattyGraph.linesMatcher || m == PattyGraph.bytesMatcher {
			linesFudgeFactor = 1
		}
		titleString := fmt.Sprintf("%s(%s)", m.name, trimmedCounts(len(m.matchCountsMap)-linesFudgeFactor))
		if m == PattyGraph.bytesMatcher {
			result.WriteString(fmt.Sprintf(m.displayColor()+"%-16.16s%1s    %4s[-:-]\n",
				titleString, m.expandGlyph(), formatBytes(m.getCount())))
		} else {
			result.WriteString(fmt.Sprintf(m.displayColor()+"%-16.16s%1s    %4s[-:-]\n",
				titleString, m.expandGlyph(), formatCounts(m.getCount())))
		}
		//result.WriteString(fmt.Sprintf(m.displayColor()+"%-5.5s %17.17s  [-:-]\n", m.name, cleanText))
		if m.displayMatchMode > 0 {
			for _, entry := range entries {
				entryString := m.entryDisplayLineFunc(entry)
				if entryString != "" {
					m.matchedDisplayCount++ // for proper list display later!
					result.WriteString(entryString)
				}
			}
		}
	}
	m.displayMatchedCache = result.String()
	return m.displayMatchedCache
}

func (m *Matcher) entryDisplayLine(entry matchEntry) string {
	return fmt.Sprintf(m.displayColor()+" %-18.18s%4s  [-:-]\n", entry.match, formatCounts(entry.count))
}

type matchEntry struct {
	match string
	count int
}

func (m *Matcher) standardEntrySort(entries []matchEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].count == entries[j].count {
			return entries[i].match < entries[j].match // Secondary sort alphabetically by match
		}
		return entries[i].count > entries[j].count
	})
}
func bytesEntrySort(entries []matchEntry) {
	// Define canonical order for size bands
	var sizeBandOrder = []string{
		"<100B",
		"100–300B",
		"300–700B",
		"700B–1K",
		"1–10K",
		"10–100K",
		"100–500K",
		"500K–1M",
		">1M",
	}

	// Build priority map
	orderMap := make(map[string]int, len(sizeBandOrder))
	for i, label := range sizeBandOrder {
		orderMap[label] = i
	}

	sort.Slice(entries, func(i, j int) bool {
		ai, aok := orderMap[entries[i].match]
		bi, bok := orderMap[entries[j].match]

		if aok && bok {
			return ai < bi // known vs known
		}
		if aok {
			return true // known first
		}
		if bok {
			return false
		}
		// fallback: alphabetical for unknown keys
		return entries[i].match < entries[j].match
	})
}

// TODO: This is horribly cobbled together :(
func linesEntrySort(entries []matchEntry) {
	// Define fixed display order for known entries
	order := map[string]int{
		"marked": 0,
		" b16":   1,
		" b17":   2,
		" b18":   3,
		" b21":   4,
		" b22":   5,
		" b24":   6,
		" b25":   7,
		" other": 8,

		"<100B":    9,
		"100–300B": 10,
		"300–700B": 11,
		"700B–1K":  12,
		"1–10K":    13,
		"10–100K":  14,
		"100–500K": 15,
		"500K–1M":  16,
		">1M":      17,
	}

	sort.Slice(entries, func(i, j int) bool {
		ai, aok := order[entries[i].match]
		bi, bok := order[entries[j].match]

		// Both entries are in the fixed order list
		if aok && bok {
			return ai < bi
		}
		// Only i is in fixed order
		if aok {
			return true
		}
		// Only j is in fixed order
		if bok {
			return false
		}
		// Neither is in the fixed list: sort alphabetically
		return entries[i].match < entries[j].match
	})
}

func (m *Matcher) previousValue() int {
	if len(m.history) == 0 {
		return 0
	}
	//sum := 0
	//limit := min(fluxDepth, len(m.history))
	//for i := 0; i < limit; i++ {
	//	sum += m.history[i]
	//}
	//return sum
	return m.history[len(m.history)-1]
}
func (m *Matcher) previousAverage() float64 {
	if len(m.history) == 0 {
		return float64(m.intervalCount)
	}

	// Determine how many entries to include in the average
	count := min(5, len(m.history))

	// Compute the sum of the last `countPlusFirst` values
	sum := 0
	for i := len(m.history) - count; i < len(m.history); i++ {
		sum += m.history[i]
	}

	// Return the average as a float
	return float64(sum) / float64(count)
}
func (m *Matcher) purgeHistory() {
	m.history = m.history[:0]
}
func (m *Matcher) tagIp(lineIp string) (string, bool) {
	// this test avoids testing lines, errs, bytes & Bots
	if m.isAddedAutobot {
		if _, exists := m.matchCountsMap[lineIp]; exists {
			return m.color, true
		}
	}
	return "", false
}
func (m *Matcher) defaultTagIpAction() bool {
	if _, exists := m.matchCountsMap[currentLine.ip]; exists {
		currentLine.captureColor = m.color
		return true
	}
	return false
}

func (m *Matcher) purgeMatchedWords() {
	m.matchCountsMap = make(map[string]int, 5)
	m.ipGroupsMap = make(map[string]int, 5)
	m.ipGroupsCountsMap = make(map[string]int, 5)
	m.displayMatchedCache = ""
	m.bytesServed = 0
}

// This varies from the original in some of the color ordering for the built in, system matchers.
// this lets the measured value also take on the red/green colors since they don't get a captureColor
func sharedSystemDisplayFunc(displayColor, stressColor string) string {
	return displayColor + "%-10.10s" + stressColor + "%4s%s" + displayColor + "%4s%1s%s[-:-]\n"
}

const browserRegexString = `(?i)\b(Chrome|CriOS|Firefox|FxiOS|Safari|DuckDuckGo|Edg|Edge|OPR|MSIE|Trident|Brave|PlayStation|Vivaldi|Baidu|SeaMonkey|Maxthon|Puffin|Silk|Sogou|Dolfin|IceCat|Iceweasel|Waterfox|K-Meleon|PaleMoon|Avant|Epiphany)[/\s]?`
const platformRegexString = `(?i)(Windows|Android|iPhone|Mac OS)`

// Instead of subclassing for the simple cases, inject behavior function pointers for specific behavior overrides
func MatcherFactory(matcherType string) *Matcher {
	switch matcherType {
	// Here for when included as part of startup, inline commands not processed here
	case "Browser":
		return newRegexMatcher(matcherType, browserRegexString)
	case "Bots":
		botsMatcher := NewPredicateMatcher("Bots")
		botsMatcher.predicateFuncs = []func() (bool, [][]string){isBotUAFaster}
		botsMatcher.setColor(PattyBotsColor)
		botsMatcher.isHistorical = true
		botsMatcher.useRegexMatchKeys = true
		botsMatcher.displayMatchMode = 1
		// The "botsMatcher" usage in the closure is on purpose
		botsMatcher.onMatchBehavior = func(_ [][]string) {
			// Simulate the regex match and capture without regex
			// NOTE: this ends up being a wider better match than regex too
			// regex within GO is very expensive compared to everything else
			// first dumb pass... if these letters can't be found none of the related strings will be there
			if matchedString, found := findBotWordFast(currentLine.replacedUserAgent); found {
				botsMatcher.matchCountsMap[matchedString]++
				botsMatcher.intervalCount++
			}
			if currentLine.captureColor == "" {
				currentLine.captureColor = PattyBotsColor
			}
			//maybePrintStack2()
		}
		botsMatcher.inlineCommandAction = func() string {
			if PattyGraph.botsMatcher.disableAutoAdd {
				return InlinePreamble + " del Bots"
			}
			return ""
		}
		return botsMatcher
	case "lines":
		linesMatcher := NewLineMatcher(matcherType, PattyLinesColor)
		linesMatcher.onMatchBehavior = func(_ [][]string) {
			linesMatcher.matchCountsMap["----"]++
			linesMatcher.intervalCount++
			band := classifyTokenBucket(currentLine.tokenBandCount)
			if band != "" {
				linesMatcher.matchCountsMap[band]++
			}

			if currentLine.captureColor != "" {
				linesMatcher.matchCountsMap["marked"]++
			}
			dlBucket := classifySizeBandKey(currentLine.bytesValue)
			linesMatcher.matchCountsMap[dlBucket]++

		}
		linesMatcher.entryDisplaySort = linesEntrySort
		linesMatcher.useRegexMatchKeys = true
		linesMatcher.tagIpAction = func() bool {
			return false
		}
		linesMatcher.entryDisplayLineFunc = linesMatcherEntryDisplay
		return linesMatcher
	case "bytes":
		bytesMatcher := NewLineMatcher(matcherType, PattyBytesColor)
		bytesMatcher.onMatchBehavior = func(_ [][]string) {
			bytesMatcher.matchCountsMap["----"] += currentLine.bytesValue
			bytesMatcher.intervalCount += currentLine.bytesValue
			//dlBucket := classifySizeBandKey(currentLine.bytesValue)
			//bytesMatcher.matchCountsMap[dlBucket]++
		}
		//bytesMatcher.entryDisplaySort = bytesEntrySort
		bytesMatcher.entryDisplayLineFunc = bytesMatcherEntryDisplay
		bytesMatcher.useRegexMatchKeys = true
		bytesMatcher.tagIpAction = func() bool {
			return false
		}
		return bytesMatcher
	case "errs":
		errsMatcher := NewLineMatcher(matcherType, PattyErrorColor)
		simplePredicate := func() (bool, [][]string) {
			return currentLine.isError(), nil
		}
		errsMatcher.displayMatchMode = 2
		errsMatcher.predicateFuncs = []func() (bool, [][]string){simplePredicate}
		errsMatcher.useRegexMatchKeys = true
		errsMatcher.onMatchBehavior = func(_ [][]string) {
			errsMatcher.matchCountsMap[currentLine.respCode]++
			errsMatcher.intervalCount++
		}
		errsMatcher.tagIpAction = func() bool {
			return false
		}
		return errsMatcher
	default:
		log.Fatalf("Unknown matcher type: %s", matcherType)
	}
	return nil
}

func newRegexMatcher(mName string, mPattern string) *Matcher {
	name := mName
	pattern := mPattern
	matcherRegex, err := regexp.Compile(pattern)
	if err != nil {
		log.Printf("Invalid regex pattern: %v\n", err)
		return nil
	}
	m := NewPredicateMatcher(name)
	m.displayMatchMode = 1
	simplePredicate := func() (bool, [][]string) {
		match := matcherRegex.FindAllStringSubmatch(currentLine.logLine, -1)
		return match != nil, match
	}
	m.onMatchBehavior = func(matchGroups [][]string) {
		if !m.useRegexMatchKeys {
			mString := ""
			gLen := len(matchGroups[0])
			if gLen == 1 {
				mString = matchGroups[0][0]
			} else if gLen >= 2 {
				mString = matchGroups[0][1]
			}
			if currentLine.ip == mString {
				m.standardMatchedAction(matchGroups)
				return
			}
		}

		m.useRegexMatchKeys = true
		for _, match := range matchGroups {
			if len(match) == 1 {
				m.matchCountsMap[match[0]]++
			} else if len(match) >= 2 {
				m.matchCountsMap[match[1]]++
			}
		}
		m.intervalCount++
	}
	m.predicateFuncs = []func() (bool, [][]string){simplePredicate}
	m.inlineCommandAction = func() string {
		return fmt.Sprintf(InlinePreamble+" add %s --regex %s", name, pattern)
	}
	return m
}

func (m *Matcher) pushStatsMsg() string {
	if PattyGraph.bytesMatcher == m {
		return ""
	}
	h, l, a := m.factoidMaxMinAvg()
	v := m.spikiness()
	return fmt.Sprintf(m.color+"%s[default]"+toolFmt("▲%s ▼%s ≈%.0f Δ%.0f"), m.name, trimmedCounts(h), trimmedCounts(l), a, v)
}
func (m *Matcher) pushStats() {
	if PattyGraph.bytesMatcher == m {
		return
	}
	pushPrintNow(m.pushStatsMsg())
}

func (m *Matcher) factoidMaxMinAvg() (int, int, float64) {
	if len(m.history) == 0 {
		return 0, 0, 0
		//return fmt.Sprintf(toolFmt("%s: no history"), m.name)
	}

	minVal := m.history[0]
	maxVal := m.history[0]
	sum := 0

	for _, v := range m.history {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
		sum += v
	}

	avg := float64(sum) / float64(len(m.history))

	//return fmt.Sprintf(toolFmt("%s hi/lo/avg:%s/%s/%.0f"),
	return maxVal, minVal, avg
}
func bytesMatcherEntryDisplay(entry matchEntry) string {
	if entry.match == "----" {
		return ""
	}
	percent := float64(entry.count) * 100 / float64(PattyGraph.intervalLines)
	return fmt.Sprintf(PattyGraph.bytesMatcher.displayColor()+" %-15.15s %5.1f%%  [-:-]\n",
		entry.match, percent)
}

func linesMatcherEntryDisplay(entry matchEntry) string {
	// This might not be used any longer?
	if entry.match == "----" {
		return ""
	}
	percent := float64(entry.count) * 100 / float64(PattyGraph.intervalLines)
	if PattyGraph.linesMatcher.displayMatchMode <= 1 && percent < 10 && entry.match != "marked" {
		return ""
	}
	return fmt.Sprintf(PattyGraph.linesMatcher.displayColor()+" %-10.10s %5s%5.1f%%  [-:-]\n", entry.match, renderColoredBar(percent), percent)
}

// renderBarBraille returns a 5-char-wide Braille bar for pct∈[0..100].
// Uses ⣿ for 2 units, ⡇ for 1 unit, space for 0.
func renderBarBraille(pct float64) string {
	const (
		half = '⡇' // '\u2847'  ⡇ dots 1,2,3,7
		full = '⣿' //'\u28FF'  ⣿ dots 1–8
	)
	// total units = 5*2 = 10, scaled by pct
	units := int(math.Round(pct / 100 * 10))
	var sb strings.Builder
	for i := 0; i < 5; i++ {
		switch {
		case units >= 2:
			sb.WriteRune(full)
			units -= 2
		case units == 1:
			sb.WriteRune(half)
			units -= 1
		default:
			sb.WriteRune(' ')
		}
	}
	return sb.String()
}

// renderColoredBar maps pct into five color bands and wraps the bar in tview tags.
func renderColoredBar(pct float64) string {
	bar := renderBarBraille(pct) // your existing 5-char Braille bar

	// Choose one of five colors:
	//  0–20%   : Green
	// 20–30%   : Lime
	// 30–40%   : Yellow
	// 40–50%   : Orange
	// 50–100%  : Red
	var color string
	switch {
	case pct < 20.0:
		color = "#00ff00" // green
	case pct < 30.0:
		color = "#7fff00" // lime
	case pct < 40.0:
		color = "#ffff00" // yellow
	case pct < 50.0:
		color = "#ff8000" // orange
	default:
		color = "#ff0000" // red
	}

	return fmt.Sprintf("[%s]%s", color, bar)
}
