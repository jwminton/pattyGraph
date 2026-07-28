// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"container/heap"
	"container/list"
	"fmt"
	"log"
	"sort"
	"strings"
)

// InterestingWordMatcher is the central stream type behind PattyGraph's
// Interesting Words, Refs, and IPs columns. It is intentionally denser than a
// simple token counter: each stream parses one slice of the current log line,
// tokenizes it, scores repeated context under log-time pressure, retains compact
// history, tracks peak entries, keeps selectable TUI rows stable, and stores a
// few source examples for inspection.
//
// The retention model is intentionally GC-like: every key has a last-seen log
// time, and each stream applies constant pressure to age out shallow or quiet
// entries. Repeated context, recent flux, and peak status keep an entry alive;
// one-off noise naturally disappears without reducing the stream to a fixed-size
// top-N snapshot.
//
// Code in this file often balances three concerns at once: hot-path matching,
// retained signal quality, and display behavior. When changing it, identify
// which concern owns the change before trying to detangle the whole type.
type InterestingWordMatcher struct {
	mName              string
	timeToLive         int
	pushIntervalCount  int             // Completed pushes represented by this stream, capped at history depth when displayed.
	peakWords          []string        // Ordered list of words that made it above threshold
	peakWordsSet       map[string]bool // Helper map to ensure unique entries in peakWords
	peakEmptyIntervals map[string]int  // Consecutive completed intervals without a hit, bounded by peakWordLimit.
	wordFrequency      map[string]*WordStats

	// Latest push-boundary Peak maintenance, retained for the next PattyLog
	// interval snapshot. Contention notifications are edge-triggered so a poor
	// scale setting cannot flood the ticker on every interval.
	peakContentionCount  int
	peakContentionActive bool
	peakRetiredCount     int
	peakRetirementGrace  int

	// printed keys eligible for selection
	currentListing []string
	selectedKey    string

	lineParser    func() string
	lineTokenizer func(parsedLine string) []string

	displayWidth int

	// caches & scratches
	titleFormat           string
	fullTitleFormat       string
	fullFormat            string
	ipScratch             *ipGroupScratch
	selectedGraphCache    string
	detailListingBuilder  strings.Builder
	peakBuilder           strings.Builder
	printedEntriesScratch []string
	// topTracker might be overkill as an LRU now, but its cost is minimal
	topTracker   *TopWordTracker
	groupTracker *TopPrefixTracker
	lruTracker   *ScoredLRUTracker

	// Lightweight counters used by factoids and display diagnostics.
	peakWordCounts   []int // rolling list of peak word counts
	totalPeakCounts  int   // sum of all counts in `peakWordCounts`
	wordStatsCreated int
}

var commonWords map[string]bool // Filters known high-frequency tokens from interesting word streams.

// Peak is bounded operational memory, not another continuously replaced top-N
// list. The runtime limit stays within PattyLog's maximum emitted entry count.
const (
	peakWordLimitDefault = 20
	peakWordLimitMin     = 1
	peakWordLimitMax     = 25
)

var peakWordLimit = peakWordLimitDefault

type peakWordCandidate struct {
	word     string
	strength float64
}

// NewInterestingWordMatcher initializes a stream with its parser, retention
// window, ranking trackers, and display scratch space.
func NewInterestingWordMatcher(matcherName string, slidingWindowDuration int) *InterestingWordMatcher {
	if commonWords == nil {
		commonWords = makeCommonWordMap(commonWordList)
	}
	m := InterestingWordMatcher{
		mName:         matcherName,
		wordFrequency: make(map[string]*WordStats, 4096),

		peakWords:          make([]string, 0, peakWordLimit),
		peakWordsSet:       make(map[string]bool, peakWordLimit),
		peakEmptyIntervals: make(map[string]int, peakWordLimit),
		timeToLive:         slidingWindowDuration,
		displayWidth:       26,
		titleFormat:        fmt.Sprintf("%%-%ds", botsDisplayWidth-19),
		// Config replay can raise peak-limit after matcher construction. Retain
		// maximum candidate capacity so Peak filtering cannot crowd out normal rows.
		topTracker:            NewTopWordTracker(InterestingWordListSize + peakWordLimitMax),
		lruTracker:            NewScoredLRUTracker(InterestingWordListSize + 50),
		printedEntriesScratch: make([]string, InterestingWordListSize+50),
	}
	m.fullTitleFormat = "[#F4F4F4]Interesting " + m.titleFormat + "[#999999]%6.6s[default:-]\n"
	m.ensureFullFormat()
	return &m
}

// Convert the slice to a map for quick lookups.
func makeCommonWordMap(words []string) map[string]bool {
	cWords := make(map[string]bool, len(words))
	for _, word := range words {
		cWords[word] = true
	}
	return cWords
}

// User-Agent and request tokenization intentionally keeps "+" and "-" intact;
// both have proven useful when comparing generated and organic traffic.
var symbolReplacer = strings.NewReplacer(
	"(", " ",
	")", " ",
	",", " ",
	"[", " ",
	"]", " ",
	":", " ",
	";", " ",
	"/", " ",
	"\\", " ",
	"?", "  ",
	"&", " ",
)

// Interesting streams share the MatcherFacade lane model with concrete matchers
// so the TUI can keep one ordered list for rendering, selection, and row math.
func (m *InterestingWordMatcher) matcherName() string {
	return m.mName
}
func (m *InterestingWordMatcher) isHistoric() bool {
	return false
}
func (m *InterestingWordMatcher) setColor(color string) {
}
func (m *InterestingWordMatcher) getCount() int { return 0 }
func (m *InterestingWordMatcher) prePush()      {}
func (m *InterestingWordMatcher) minMaxHistory() (int, int) {
	return 0, 0
}

func (m *InterestingWordMatcher) setPurgeInterval(timeToLive int) {
	m.timeToLive = timeToLive
}

// renderSparklineRow was formerly called displayString.
func (m *InterestingWordMatcher) renderSparklineRow() string {
	if PattyGraph.selectedInterestingMatcher == m && m.selectedKey != "" {
		sparkBuilder := strings.Builder{}
		fv := 0
		stats := m.wordFrequency[m.selectedKey]
		var sparkSlice []int
		if stats == nil && m.ipScratch != nil {
			// This is the groupedIP spoofed WordStats made just for this display
			stats = m.ipScratch.prefixStats[m.selectedKey]
			if stats != nil {
				sparkSlice = m.ipScratch.prefixHistorAggregateBufs[m.selectedKey].Slice()
				if len(sparkSlice) > 0 {
					fv = sparkSlice[len(sparkSlice)-1]
				}
			}
		} else if stats != nil {
			sparkSlice = stats.historySlice()
			fv = stats.historyBuf.Latest()
		}
		if stats == nil {
			return fmt.Sprintf("[default]%-10.10s   -:   -|[-:-]\n", m.selectedKey)
		}

		lineColor := "[default:-]"
		if stats.source != nil && stats.source.captureColor != "" {
			lineColor = stats.source.captureColor
		}

		sparkBuilder.WriteString(fmt.Sprintf("%s%-10.10s%4s:%4s|", lineColor, m.selectionKey(),
			formatCounts(stats.count), formatCounts(fv)))
		wordBottom := 0
		wordTop := max(lastMonitorMaxBuf.Latest(), fv*11/10)

		// CACHE THIS? -- NO for ip groups its an aggregate that's changing with each update
		sparkBuilder.WriteString(sparklineFromArray(wordBottom, wordTop, sparkSlice)) // gets reversed
		sparkBuilder.WriteString("[-:-]\n")

		//// careful here bc nil access when things are selected and things get timed out
		//line := ""
		//if stats.source != nil {
		//	line = stats.source.logLine
		//	// IP Groups don't have a log line bc its a faux entry for the aggregate
		//	// use last line if tabbed from first tab view
		//	if PattyGraph.secondaryView == SecondaryViewPrimeFlux {
		//		if stats.firstIntervalLine != nil && stats.firstIntervalLine.logLine != "" {
		//			line = stats.firstIntervalLine.logLine
		//		} else {
		//			line = ""
		//		}
		//	} else if PattyGraph.secondaryView > SecondaryViewPrimeFlux {
		//		if stats.lastLine != nil && stats.lastLine.logLine != "" {
		//			line = stats.lastLine.logLine
		//		}
		//	}
		//}
		//
		//sparkBuilder.WriteString(
		//	prettyPrintLogLine(line,
		//		PattyGraph.selectedInterestingMatcher.selectedKey,
		//		startingLineColor))
		return sparkBuilder.String()
	}
	return ""
}
func (m *InterestingWordMatcher) displayLogLine() string {
	if PattyGraph.selectedInterestingMatcher == m && m.selectedKey != "" {
		sparkBuilder := strings.Builder{}
		stats := m.wordFrequency[m.selectedKey]
		if stats == nil && m.ipScratch != nil {
			// This is the groupedIP spoofed WordStats made just for this display
			stats = m.ipScratch.prefixStats[m.selectedKey]
		}
		if stats == nil {
			return ""
		}
		lineColor := "[default:-]"
		if stats.source != nil && stats.source.captureColor != "" {
			lineColor = stats.source.captureColor
		}
		startingLineColor := lineColor

		// careful here bc nil access when things are selected and things get timed out
		line := ""
		if stats.source != nil {
			line = stats.source.logLine
			// IP Groups don't have a log line bc its a faux entry for the aggregate
			// use last line if tabbed from first tab view
			switch PattyGraph.secondaryView {
			case SecondaryViewPrimeFlux:
				line = stats.firstIntervalLogLine
				//if stats.firstIntervalLine != nil && stats.firstIntervalLine.logLine != "" {
				//	line = stats.firstIntervalLine.logLine
				//} else {
				//	line = ""
				//}
			case SecondaryViewPattyFactor:
				// The first-seen source is already selected above.
			default:
				//if stats.lastLine != nil && stats.lastLine.logLine != "" {
				//	line = stats.lastLine.logLine
				//}
				line = stats.lastLogLine
			}
		}

		sparkBuilder.WriteString(
			prettyPrintLogLine(line,
				PattyGraph.selectedInterestingMatcher.selectedKey,
				startingLineColor))
		return sparkBuilder.String()
	}
	return ""
}

func prettyPrintLogLine(line, selectedKey, lineColor string) string {
	sparkBuilder := strings.Builder{}
	sparkBuilder.WriteString(lineColor)
	for i := 0; i < len(line); i += PattyPrintWidth {
		end := i + PattyPrintWidth
		if end > len(line) {
			end = len(line)
		}
		baseText := line[i:end]
		// if "match" bridges a PattyPrintWidth boundary, fine, we just won't highlight
		if selectedKey != "" {
			if idx := strings.Index(baseText, selectedKey); idx != -1 {
				// Inject highlight tags
				prefix := baseText[:idx]
				match := baseText[idx : idx+len(selectedKey)]
				suffix := baseText[idx+len(selectedKey):]
				baseText = prefix + "[white]" + match + lineColor + suffix
			}
		}

		sparkBuilder.WriteString(baseText)
		sparkBuilder.WriteString("\n")
	}
	return sparkBuilder.String()
}

func ipsParseLine() string {
	return currentLine.ip
}
func refsParseLineFast() string {
	ref := currentLine.referer

	if len(ref) == 0 || ref == "-" || ref == " " {
		return "--empty--"
	}

	if i := strings.Index(ref, "//"); i != -1 {
		// Avoids reassignment, preserves original input if needed
		return symbolReplacer.Replace(ref[i+2:])
	}

	return symbolReplacer.Replace(ref)
}

func wordsParseLine() string {
	// User-Agent content will come from pre-processed inputLine.userAgentTokens during token processing
	return symbolReplacer.Replace(currentLine.request)
}

var tokenScratch = make([]string, 0, 100)

func tokensForIps(parseLine string) []string {
	result := tokenScratch[:1]
	result[0] = parseLine
	return result
}

func tokensForRefs(parseLine string) []string {
	rawWords := fastFieldsASCIIBuf(parseLine, &refFieldsBuf) // tokens for refs
	if cap(tokenScratch) < len(rawWords) {
		tokenScratch = make([]string, 0, len(rawWords)*2)
	}
	result := tokenScratch[:0]
	for _, word := range rawWords {
		if len(word) >= DefaultMinWordLength {
			result = append(result, stringInterner.Intern(word))
		}
	}
	return result
}

func tokensForWords(parseLine string) []string {
	// Split the logLine into words
	rawWords := fastFieldsASCIIBuf(parseLine, &reqFieldsBuf) // tokens for words
	// Filter out uninteresting & small words
	if cap(tokenScratch) < len(rawWords)+len(currentLine.userAgentTokens) {
		tokenScratch = make([]string, 0, (len(rawWords)+len(currentLine.userAgentTokens))*2)
	}
	words := tokenScratch[:0]
	for _, word := range currentLine.userAgentTokens {
		if isInteresting(word) {
			// userAgent tokens have already been interned
			words = append(words, word)
		}
	}
	PattyGraph.totalAgentTokens += uint64(len(words))

	for _, word := range rawWords {
		if isInteresting(word) {
			words = append(words, stringInterner.Intern(word))
		}
	}
	if len(words) == 0 {
		// its the most generic request possible "/" and filtered userAgent content
		words = append(words, filteredToken)
	}
	return words
}

// processes a logLine of text to find and intervalCount interesting words
func (m *InterestingWordMatcher) match() bool {
	poolGetsStart := poolGets
	// Making these be assigned func's was a huge win for logic and efficiency
	words := m.lineTokenizer(m.lineParser())

	// invariant for this call
	// example tests to keep logic identical
	// lc 20 ttl 5 lastSeen 15 = lives(skip new creation)
	// lc 21 ttl 5 lastSeen 15 = purge(new creation)
	//matcherDurationInt := m.timeToLive
	limit := logicalCycles - m.timeToLive
	// Expiry is evaluated here because the TTL is matcher-specific log time.
	for _, word := range words {
		stats, exists := m.wordFrequency[word]
		// if new or should be timed out
		if !exists || stats.lastSeenTic < limit {
			if exists {
				// Recycle expired stats and start a fresh entry for this token.
				recycleWordStats(stats)
			}
			stats := newWordStats()
			m.wordFrequency[word] = stats
			m.wordStatsCreated++
			m.lruTracker.MarkSeen(word, 1)
			if m.ipScratch != nil {
				stats.retainAgentTokenBaseline(currentLine.userAgentTokens)
				m.ipScratch.Add(currentLine.ip, currentLine.ipPrefix)
			}
			continue
		}

		stats.count++
		stats.bytes += uint64(currentLine.bytesValue)
		stats.primeFlux++
		m.lruTracker.MarkSeen(word, stats.primeFlux)
		stats.burstiCache = 0
		stats.lastLogLine = currentLine.logLine
		if stats.firstIntervalLogLine == "" {
			stats.firstIntervalLogLine = currentLine.logLine
		}
		stats.lastSeenTic = logicalCycles
		stats.lastStatus = currentLine.respCode
		if currentLine.captureColor != "" && (!firstColorWins || stats.source.captureColor == "") {
			stats.source.captureColor = currentLine.captureColor
			stats.source.captureMatcher = currentLine.captureMatcher
		}
		previousWeight := float64(stats.count)
		newWeight := previousWeight + 1
		stats.agentDeltaMetric = ((stats.agentDeltaMetric * previousWeight) + currentLine.userAgentDelta) / newWeight
	}
	poolGetsStop := poolGets
	poolGetsThisCall := poolGetsStop - poolGetsStart
	poolGetsPerMatcherMap[poolGetsThisCall]++

	return false // This is ignored for wordMatchers. Bool only here for autobot matching and this is a shared interface
}

// push (aka flagInterestingWords) identifies words that exceed the threshold and are "interesting"
// threshold is globalavg
func (m *InterestingWordMatcher) push() {
	m.pushIntervalCount++
	m.selectedGraphCache = ""
	m.peakContentionCount = 0
	m.peakRetiredCount = 0
	m.peakRetirementGrace = 0
	// invariants
	nDenom := m.normalizedDenominator()
	limit := logicalCycles - m.timeToLive
	candidates := make([]peakWordCandidate, 0, peakWordLimit)
	retired := make(map[string]bool)
	for word, stats := range m.wordFrequency {
		oldCount := stats.count
		// Remove entries outside the sliding window to keep data fresh
		// but make push wait just a little longer so we're not stepping on match timing/pruning
		//if logicalCycles-stats.lastSeenTic > timeToLive {
		if stats.lastSeenTic < limit {
			if m.peakWordsSet[word] {
				// Peak history continues across ordinary staleness so absence is
				// visible and can eventually retire the protected entry.
				stats.push()
				if m.recordPeakInterval(word, oldCount) {
					retired[word] = true
					m.removeWordStats(word, stats)
				}
			} else {
				m.removeWordStats(word, stats)
			}
			continue
		}
		stats.push()
		if m.peakWordsSet[word] {
			if m.recordPeakInterval(word, oldCount) {
				retired[word] = true
				m.removeWordStats(word, stats)
			}
			continue
		}

		// Don't make any Peak decisions unless we've completed enough history.
		if m.pushIntervalCount >= pattyGracePeriod {
			if stats.historyLength() >= pattyGracePeriod {
				if strength, eligible := m.peakEligibility(stats, oldCount, nDenom); eligible {
					candidates = append(candidates, peakWordCandidate{word: word, strength: strength})
				}
			}
		}
	}
	m.finishPeakRetirements(retired)
	m.admitPeakCandidates(candidates)
	m.updatePeakWordStats()
	m.wordStatsCreated = 0
}

func (m *InterestingWordMatcher) removeWordStats(word string, stats *WordStats) {
	// This delete and m.ipScratch.Remove MUST be kept in sync.
	delete(m.wordFrequency, word)
	if m.ipScratch != nil {
		m.ipScratch.Remove(stats.source.ip, stats.source.ipPrefix)
	}
	m.lruTracker.Delete(word)
	recycleWordStats(stats)
}

func (m *InterestingWordMatcher) peakEligibility(stats *WordStats, intervalCount int, normalizedDenominator float64) (float64, bool) {
	if m.mName == "ips" {
		burst := stats.burstiness()
		eligible := (intervalCount > 10 && burst >= 1.0) ||
			float64(intervalCount) > (lastLinesBuf.nFluxAvg(fluxDepth)/10)
		return float64(intervalCount), eligible
	}

	strength := stats.normalized() / normalizedDenominator
	return strength, strength >= 1.0
}

func (m *InterestingWordMatcher) recordPeakInterval(word string, intervalCount int) bool {
	if intervalCount > 0 {
		m.peakEmptyIntervals[word] = 0
		return false
	}

	empty := m.peakEmptyIntervals[word] + 1
	m.peakEmptyIntervals[word] = empty
	if empty < pattyGracePeriod {
		return false
	}

	delete(m.peakWordsSet, word)
	delete(m.peakEmptyIntervals, word)
	return true
}

func (m *InterestingWordMatcher) finishPeakRetirements(retired map[string]bool) {
	if len(retired) == 0 {
		return
	}

	kept := m.peakWords[:0]
	for _, word := range m.peakWords {
		if !retired[word] {
			kept = append(kept, word)
		}
	}
	m.peakWords = kept
	m.peakRetiredCount = len(retired)
	m.peakRetirementGrace = pattyGracePeriod
	if !doRandomFact {
		return
	}
	_, _ = pushFactSnapshotNow("interesting.peakRetirement", []string{
		peakStreamLabel(m.mName),
		fmt.Sprintf("%d", len(retired)),
	})
}

func (m *InterestingWordMatcher) admitPeakCandidates(candidates []peakWordCandidate) {
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].strength == candidates[j].strength {
			return candidates[i].word < candidates[j].word
		}
		return candidates[i].strength > candidates[j].strength
	})

	available := peakWordLimit - len(m.peakWords)
	if available < 0 {
		available = 0
	}
	admitted := min(available, len(candidates))
	for _, candidate := range candidates[:admitted] {
		m.peakWords = append(m.peakWords, candidate.word)
		m.peakWordsSet[candidate.word] = true
		m.peakEmptyIntervals[candidate.word] = 0
	}

	m.updatePeakContention(len(candidates) - admitted)
}

func (m *InterestingWordMatcher) updatePeakContention(count int) {
	m.peakContentionCount = count
	if count == 0 {
		m.peakContentionActive = false
		return
	}
	if m.peakContentionActive {
		return
	}
	if !doRandomFact {
		return
	}

	m.peakContentionActive = true
	_, _ = pushFactSnapshotNow("interesting.peakContention", []string{
		peakStreamLabel(m.mName),
		fmt.Sprintf("%d", count),
		m.peakContentionTickerGuidance(),
	})
}

func peakStreamLabel(name string) string {
	switch name {
	case "ips":
		return "IPs"
	case "refs":
		return "Refs"
	default:
		return "Words"
	}
}

func (m *InterestingWordMatcher) peakContentionTickerGuidance() string {
	if m.mName == "ips" {
		return "review/purge"
	}
	return "lower scale"
}

func (m *InterestingWordMatcher) peakContentionGuidance() string {
	if m.mName == "ips" {
		return "review or purge Peak membership"
	}
	return "lower scale for more selective Peak membership"
}

func setPeakWordLimit(requested int) (effective int, changed bool, clamped bool) {
	effective = requested
	if effective < peakWordLimitMin {
		effective = peakWordLimitMin
	} else if effective > peakWordLimitMax {
		effective = peakWordLimitMax
	}
	clamped = effective != requested
	changed = effective != peakWordLimit
	peakWordLimit = effective
	return effective, changed, clamped
}

func reportPeakWordLimitUpdate(requested int, effective int, changed bool, clamped bool) {
	if clamped {
		_, _ = pushFactSnapshotNow("settings.peak-limit-clamped", []string{
			fmt.Sprintf("%d", requested),
			fmt.Sprintf("%d", effective),
		})
	}
	if changed {
		pushFactNow("settings.peak-limit", nil)
	}
}

// Cache callers should compute this once per display cycle, not once per entry.
func (m *InterestingWordMatcher) normalizedDenominator() float64 {
	if PattyGraph.intervalsCompleted == 0 {
		return PattyGraph.linesMatcher.previousAverage() / 10.0
	}
	return (float64(lastLinesBuf.nFlux(fluxDepth)) + (2 * PattyGraph.linesMatcher.previousAverage())) / float64((fluxDepth+2)*10.0) // go go gadgetcompiler
	//return (float64(lastLastLinesMax+lastLinesMax) + (2 * PattyGraph.linesMatcher.previousAverage())) / (4 * 10.0) // go go gadgetcompiler
	//return m.pushVal * 10
}

type ipGroupScratch struct {
	mEntries                 map[string]int
	mColors                  map[string]string
	mMatchers                map[string]string
	prefixCounts             map[string]int
	prefixColors             map[string]string
	prefixMatchers           map[string]string
	prefixDepths             map[string]int
	prefixBursts             map[string]float64
	prefixBytes              map[string]uint64
	prefixDeltas             map[string]float64
	prefixMembers            map[string]int
	prefixStats              map[string]*WordStats
	prefixFirstPlusCounts    map[string]int
	prefixFirstLines         map[string]string
	prefixLastLines          map[string]string
	prefixFirstIntervalLines map[string]string
	//prefixHistoryBufs        map[string]*ringBuffer

	prefixHistorAggregateBufs map[string]*ringSeriesAccumulator

	// prefix → set of member IPs (existence only)
	prefixToIPs map[string]map[string]struct{} // does NOT get cleared!
	// derived state for fast ActivePrefixes:
	activePrefixes     map[string]struct{} // only prefixes with count ≥ threshold
	activePrefixCounts map[string]int
	// metrics
	activePrefixesCountMetric int
}

func (s *ipGroupScratch) Clear() {
	for k := range s.prefixFirstPlusCounts {
		delete(s.prefixFirstPlusCounts, k)
	}
	for k := range s.prefixCounts {
		delete(s.prefixCounts, k)
	}
	for k := range s.mEntries {
		delete(s.mEntries, k)
	}
	for k := range s.mColors {
		delete(s.mColors, k)
	}
	for k := range s.mMatchers {
		delete(s.mMatchers, k)
	}
	for k := range s.prefixColors {
		delete(s.prefixColors, k)
	}
	for k := range s.prefixMatchers {
		delete(s.prefixMatchers, k)
	}
	for k := range s.prefixDepths {
		delete(s.prefixDepths, k)
	}
	for k := range s.prefixBursts {
		delete(s.prefixBursts, k)
	}
	for k := range s.prefixBytes {
		delete(s.prefixBytes, k)
	}
	for k := range s.prefixDeltas {
		delete(s.prefixDeltas, k)
	}
	for k := range s.prefixMembers {
		delete(s.prefixMembers, k)
	}
	for k := range s.prefixLastLines {
		delete(s.prefixLastLines, k)
	}
	for k := range s.prefixFirstIntervalLines {
		delete(s.prefixFirstIntervalLines, k)
	}
	for k, _ := range s.prefixStats {
		// this will leak objects that were going to be gc'd anyway
		// on the order of display cycles (hundreds not millions)
		//prefixRecycleCount++
		//recycleWordStats(stats)
		delete(s.prefixStats, k)
	}
	//for k, _ := range s.prefixHistoryBufs {
	//	//putRingBuffer(buf)
	//	delete(s.prefixHistoryBufs, k)
	//}
	for k, buf := range s.prefixHistorAggregateBufs {
		poolAccumulator(buf)
		delete(s.prefixHistorAggregateBufs, k)
	}
}

//var prefixRecycleCount = 0

/*
	After much profiling and optimizing, this is the most intense method of the whole tool.
	Aggregate information is gathered here live each display cycle for accuracy. The whole
	list of ips is never fully swept thanks to the prefix membership map maintained on
	insert/delete. The ip prefixes are swept and their members iterated over for aggregates.
	Optimizations that avoid this per-display summation have been rife with inaccuracies from
	various sources of logical failures.
*/
// This path is intentionally explicit because it aggregates prefix state across
// active IPs during display. The topN/heap approach keeps the per-display work
// bounded enough for the TUI.
func (m *InterestingWordMatcher) displayIpGroups() (string, []string) {
	defer m.groupTracker.Reset()

	scratch := m.ipScratch // caller resets ipScratch
	prefixGroups := scratch.prefixMembers
	sortedGroups, selectedGroup := m.sortedIpGroups()

	// Sort the slice by intervalCount in descending order
	// use peak group membership as the primary sort and then
	// do a secondary sort between the peak/non-peak by intervalCount
	sort.Slice(sortedGroups, func(i, j int) bool {
		if sortedGroups[i].countPlusFirst == sortedGroups[j].countPlusFirst {
			return sortedGroups[i].prefix < sortedGroups[j].prefix
		}
		return sortedGroups[i].countPlusFirst > sortedGroups[j].countPlusFirst
	})

	// Keep the displayed prefix group list compact enough for the TUI pane.
	if len(sortedGroups) > 12 {
		sortedGroups = sortedGroups[:12]
		if selectedGroup != nil {
			found := false
			for _, g := range sortedGroups {
				if g.prefix == selectedGroup.prefix {
					found = true
					break
				}
			}
			if !found {
				sortedGroups = append(sortedGroups, *selectedGroup)
			}
		}
	}

	// Make a map of <15 prefixes we care about to avoid a linear probe for membership in the following loop
	//groupPrefixes := make(map[string]struct{}, 20)
	//for _, group := range sortedGroups {
	//	groupPrefixes[group.prefix] = struct{}{}
	//}

	// POST CUT!!!! only <15 in sortedGroups now, whew
	// now that we're the top 10 or so, get the expensive or niche data
	//for ipAddr, stats := range m.wordFrequency {
	//if _, ok := groupPrefixes[stats.source.ipPrefix]; !ok {
	//	continue
	//}
	////prefix := stats.source.ipPrefix
	//for _, prefix := range m.ipScratch.ActivePrefixes(15) {
	for _, prefixMarker := range sortedGroups {
		prefix := prefixMarker.prefix
		for _, ipAddr := range m.ipScratch.ActivePrefixMembers(prefix) {
			stats := m.wordFrequency[ipAddr]
			switch PattyGraph.secondaryView {
			// burstiness was worse before it was a cached value
			case SecondaryViewPattyFactor:
				// burstiness
				scratch.prefixBursts[prefix] += stats.burstiness()
			case SecondaryViewAgentDelta:
				//secondaryKeyOut.WriteString(fmt.Sprintf("%5d%%", int(stats.agentDeltaMetric*100)))
				scratch.prefixDeltas[prefix] += stats.agentDeltaMetric
				//secondaryString = fmt.Sprintf("%5d%%", int(float64(scratch.prefixDeltas[group.prefix])/float64(scratch.prefixMembers[group.prefix]))*100)
			case SecondaryViewBytes:
				// burstiness
				scratch.prefixBytes[prefix] += stats.bytes
			}

			scratch.mEntries[ipAddr] = stats.count

			if scratch.prefixFirstLines[prefix] == "" {
				scratch.prefixFirstLines[prefix] = stats.source.logLine
			}
			if stats.lastLogLine != "" {
				scratch.prefixLastLines[prefix] = stats.lastLogLine
			}
			if scratch.prefixFirstIntervalLines[prefix] == "" && stats.firstIntervalLogLine != "" {
				scratch.prefixFirstIntervalLines[prefix] = stats.firstIntervalLogLine
			}

			if stats.source.captureColor != "" && scratch.mColors[ipAddr] == "" {
				scratch.mColors[ipAddr] = stats.source.captureColor
				scratch.mMatchers[ipAddr] = stats.source.captureMatcher
				if _, exists := scratch.prefixColors[prefix]; !exists {
					scratch.prefixColors[prefix] = stats.source.captureColor
					scratch.prefixMatchers[prefix] = stats.source.captureMatcher
				}
			}
			// Minimum info needed. Expensive info is processed later post-cut
			scratch.prefixMembers[prefix] += 1
			src := stats.historyBuf
			scratch.prefixDepths[prefix] = max(scratch.prefixDepths[prefix], src.Len())
			agg, exists := scratch.prefixHistorAggregateBufs[prefix]
			if !exists {
				scratch.prefixHistorAggregateBufs[prefix] = accumulatorFor(src)
			} else {
				agg.MergeFromRing(src)
			}
		}
	}

	var result = strings.Builder{}
	result.Grow(10 * m.displayWidth)
	var printedEntries []string
	// Print the sorted results
	selectedMatcherColor := matcherSelectionColor()
	iCount := m.pushIntervalCount
	if iCount > DefaultHistoryDepth {
		iCount = DefaultHistoryDepth
	}
	// Render prefix groups before individual IPs so shared subnet behavior is
	// visible without expanding every member.
	for _, group := range sortedGroups {
		if prefixGroups[group.prefix] < peakIpThreshold {
			continue
		}
		var secondaryString string
		// Some of this is an estimation and it can't really be pulled out bc its a lot of 2nd level derived data.
		switch PattyGraph.secondaryView {
		case SecondaryViewPattyFactor:
			// burstiness
			secondaryString = fmt.Sprintf("%6.2f", float64(scratch.prefixBursts[group.prefix])/float64(scratch.prefixMembers[group.prefix]))
		case SecondaryViewPrimeFlux:
			// countPlusFirst
			secondaryString = fmt.Sprintf("%6d", group.countPlusFirst)
		case SecondaryViewHistoryDepth:
			secondaryString = fmt.Sprintf("[%d]", scratch.prefixDepths[group.prefix])
		case SecondaryViewAgentDelta:
			//secondaryKeyOut.WriteString(fmt.Sprintf("%5d%%", int(stats.agentDeltaMetric*100)))
			secondaryString = fmt.Sprintf("%5d%%", int(100*scratch.prefixDeltas[group.prefix]/float64(scratch.prefixMembers[group.prefix])))
		case SecondaryViewSparkline:
			//secondaryString = fmt.Sprintf("%-6s", miniReverseSparklineFromArray(*scratch.prefixHistoryBufs[group.prefix].ReverseSlice()))
			secondaryString = fmt.Sprintf("%-6s", miniReverseSparklineFromArray(scratch.prefixHistorAggregateBufs[group.prefix].ReverseSlice()))
		case SecondaryViewBytes:
			// Bytes this interval
			secondaryString = fmt.Sprintf("%6s", formatBytes64(scratch.prefixBytes[group.prefix]))
		}

		groupString := fmt.Sprintf("%s*.*(%d)", group.prefix, prefixGroups[group.prefix])
		// Temporary display-only stats let prefix groups reuse matched-entry formatting.
		fakeStat := &WordStats{
			historyBuf: &ringBuffer{},
			//historyBuf: scratch.prefixHistorAggregateBufs[group.prefix].ringClone(),
			//historyBuf:  scratch.prefixHistoryBufs[group.prefix],
			forcedColor: scratch.prefixColors[group.prefix],
			lastStatus:  "200",
		}
		//fakeStat := blankWordStats()
		//fakeStat.forcedColor = scratch.prefixColors[group.prefix]
		//fakeStat.lastStatus = "200"
		scratch.prefixHistorAggregateBufs[group.prefix].cloneInto(fakeStat.historyBuf)
		fakeEntry := wordEntry{
			key:  group.prefix,
			word: groupString,
			stat: fakeStat,
		}
		result.WriteString(m.writeMatchedEntry(&fakeEntry,
			selectedMatcherColor,
			pattyMonoColorForInt(scratch.prefixDepths[group.prefix]),
			secondaryString))

		printedEntries = append(printedEntries, group.prefix)
		//recycleWordStats(fakeStat)
	}
	// could consolidate with the above, but its only 10 entries, figure it out later.
	// making faux WordStats & lineSource for renderSparklineRow needs later
	for _, group := range sortedGroups {
		//fauxSource := &lineSource{
		//	captureColor: scratch.prefixColors[group.prefix],
		//	logLine:      scratch.prefixFirstLines[group.prefix],
		//}
		//groupCount := group.count
		//firstHistory := 0
		//if scratch.prefixHistoryBufs[group.prefix] != nil && scratch.prefixHistoryBufs[group.prefix].Len() > 0 {
		//	//firstHistory = scratch.prefixHistoryBufs[group.prefix].nFlux(fluxDepth)
		//	firstHistory = scratch.prefixHistoryBufs[group.prefix].Latest()
		//}
		//nws := blankWordStats()
		//nws := &WordStats{
		//	historyBuf: &ringBuffer{},
		//	//source:     &lineSource{},
		//}
		nwsBuf := &ringBuffer{}
		ls := &lineSource{}
		ls.captureColor = scratch.prefixColors[group.prefix]
		ls.captureMatcher = scratch.prefixMatchers[group.prefix]
		ls.logLine = scratch.prefixFirstLines[group.prefix]
		nws := &WordStats{
			count: group.count,
			//historyBuf: scratch.prefixHistorAggregateBufs[group.prefix].ringClone(), // needs ringBuffer copy instead
			historyBuf:           nwsBuf, // needs ringBuffer copy instead
			firstIntervalLogLine: scratch.prefixFirstIntervalLines[group.prefix],
			lastLogLine:          scratch.prefixLastLines[group.prefix],
			forcedColor:          scratch.prefixColors[group.prefix],
			primeFlux:            group.countPlusFirst,
			source:               ls,
		}
		scratch.prefixHistorAggregateBufs[group.prefix].cloneInto(nwsBuf)
		scratch.prefixStats[group.prefix] = nws
	}
	return result.String(), printedEntries
}

func (m *InterestingWordMatcher) sortedIpGroups() ([]prefixCount, *prefixCount) {
	scratch := m.ipScratch // caller resets ipScratch

	// GOLD STANDARD BRUTE FORCE THAT ALWAYS WORKS
	// optimizations must simulate at least this much for working subsets.
	//for _, stats := range m.wordFrequency {
	//	prefix := stats.source.ipPrefix
	//	scratch.prefixFirstPlusCounts[prefix] += stats.primeFlux
	//	scratch.prefixCounts[prefix] += stats.count
	//}
	for _, prefix := range m.ipScratch.ActivePrefixes() { // sortedIpGroups
		for _, ip := range m.ipScratch.ActivePrefixMembers(prefix) { // sortedIpGroups
			stats, ok := m.wordFrequency[ip]
			if !ok {
				continue
			}
			scratch.prefixFirstPlusCounts[prefix] += stats.primeFlux
			scratch.prefixCounts[prefix] += stats.count
		}
	}

	var selectedGroup *prefixCount
	groupTest := ""
	if PattyGraph.selectedInterestingMatcher == m {
		if m.selectedKey != "" {
			groupTest = m.selectedKey
		}
	}

	// scratch.prefixCounts can be thousands of entries. Cutting it down asap is important
	for prefix, countPlusFirst := range scratch.prefixFirstPlusCounts {
		//count := scratch.prefixCounts[prefix]
		if m.groupTracker.ShouldConsider(countPlusFirst) || prefix == groupTest {
			newCount := prefixCount{prefix,
				countPlusFirst,
				scratch.prefixCounts[prefix]}
			if prefix == groupTest {
				selectedGroup = &newCount // pick off selection to add back in later if needed
			}
			m.groupTracker.MaybeAdd(newCount)
		}
	}
	// sortedGroups now has TopN Sort size: 15
	return m.groupTracker.Top(), selectedGroup
}

// This has been left mostly un-optimized. Logic is a little messy but its correct and it never shows up on profiles
// Consolidate printing logic makes this even more uninteresting in the grand scheme (renderDetailListing and ipGroups
// are far worse
func (m *InterestingWordMatcher) displayPeakWords() (string, []string) {
	if len(m.peakWords) == 0 {
		return "", nil
	}
	defer m.peakBuilder.Reset()
	// Collect words and their ratios
	entries := make([]wordEntry, 0, len(m.peakWords))
	for _, word := range m.peakWords {
		count := 0
		if stat, ok := m.wordFrequency[word]; ok && stat != nil {
			count = stat.historyTotal()
		}
		entries = append(entries, wordEntry{word: word, count: count, stat: m.wordFrequency[word]})
	}

	// Sort entries by ratio in descending order
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].count > entries[j].count
	})

	// Build the result string from sorted entries
	result := m.peakBuilder
	var printedEntries []string
	nDenom := m.normalizedDenominator()
	selectedMatcherColor := matcherSelectionColor()

	for _, entry := range entries {
		// hugely simplified by refactoring and calling same entry printing

		secondaryInfo := m.secondaryEntryMetric(entry.stat, nDenom)
		result.WriteString(m.writeMatchedEntry(&entry, selectedMatcherColor, PattyOrange, secondaryInfo))
		printedEntries = append(printedEntries, entry.word)
	}
	return result.String(), printedEntries
}

// wordEntry is the compact sortable view of WordStats used by top-N selection.
type wordEntry struct {
	word       string
	key        string
	count      int // really the underlying current + history[0]
	stat       *WordStats
	hasCapture bool
}

func (m *InterestingWordMatcher) selected() bool {
	return PattyGraph.selectedInterestingMatcher == m
}

func (m *InterestingWordMatcher) asInlineCommand() string {
	return ""
}

//var printedEntriesScratch []string = make([]string, InterestingWordListSize*2)

// renderDetailListing builds one selectable Words, Refs, or IPs column. This
// was the hottest display path until displayIpGroups took that title; retaining
// a bounded candidate set keeps its work manageable.
// Formerly called displayMatched.
func (m *InterestingWordMatcher) renderDetailListing() string {
	defer m.detailListingBuilder.Reset()
	defer m.topTracker.Reset()

	entries := m.topWordEntries()
	// Sort entries
	// The idea is to
	//    * primary sort on peakWordEntry.countPlusFirst
	//    * secondary grouped by color
	//    * lastly keep things alphabetical so they
	//      don't jump around needlessly within the same grouping
	//sort.Slice(entries, func(i, j int) bool {
	//	if entries[i].count == entries[j].count {
	//		// Primary sort: Entries with captureColor come before...
	//		if entries[i].hasCapture != entries[j].hasCapture {
	//			return entries[i].hasCapture
	//		}
	//		// Secondary sort: Alphabetical order of words within grouping
	//		return entries[i].word < entries[j].word
	//	}
	//	// Primary Sort by peakWordEntry.countPlusFirst descending
	//	return entries[i].count > entries[j].count
	//})
	// efficient redo of the above:
	sort.Slice(entries, func(i, j int) bool {
		ci, cj := entries[i].count, entries[j].count
		if ci != cj {
			return ci > cj
		}
		hi, hj := entries[i].hasCapture, entries[j].hasCapture
		if hi != hj {
			return hi // true < false in Go
		}
		return entries[i].word < entries[j].word
	})

	result := m.detailListingBuilder
	printedCount := 0
	// ***** Title logLine *****
	extra := fmt.Sprintf("(%s)", strings.TrimSpace(formatCounts(len(m.wordFrequency))))
	result.WriteString(fmt.Sprintf(m.fullTitleFormat, m.mName, extra))

	// printedEntries is a cache of what logical key was responsible for what was actually printed
	// in what order so that later when that key is "selected", it is printedEntries that is really
	// getting selected from, not anything displayed (markup + secondary)
	printedEntries := m.printedEntriesScratch[:0]

	if m.ipScratch != nil {
		resultString, groupEntries := m.displayIpGroups()
		result.WriteString(resultString)
		printedEntries = append(printedEntries, groupEntries...)
	}
	resultString, peakEntries := m.displayPeakWords()
	result.WriteString(resultString)
	printedEntries = append(printedEntries, peakEntries...)

	intervalCount := m.pushIntervalCount
	if intervalCount > DefaultHistoryDepth {
		intervalCount = DefaultHistoryDepth
	}
	listEntryLimit := len(m.peakWordsSet) + InterestingWordListSize
	// invariant for this call that's best done outside the following loop
	nDenom := max(1, m.normalizedDenominator())
	selectedMatcherColor := matcherSelectionColor()

	for _, entry := range entries {
		if !m.peakWordsSet[entry.word] { // skip peak words since they were covered in displayPeakWords
			result.WriteString(m.writeMatchedEntry(
				&entry,
				selectedMatcherColor,
				entry.stat.pattyMonoColor(),
				m.secondaryEntryMetric(entry.stat, nDenom)))
			printedEntries = append(printedEntries, entry.word)
			printedCount++
			if printedCount >= listEntryLimit {
				break
			}
		}
	}
	m.currentListing = printedEntries
	return result.String()
}

// There's some battling lrus here due to code evolution. The overhead is minimal so I left it.
// Using heap/lru to cut down the number of entries to process from thousands to <200 was a huge win
func (m *InterestingWordMatcher) topWordEntries() []wordEntry {
	// caller resets topTracker!
	// h, limit and the ipScratch test have all been pulled out and logic is repeated below all for performance reasons
	h := m.topTracker.h
	limit := m.topTracker.limit
	scratch := m.ipScratch
	topList := m.lruTracker.TopN(110)
	if scratch != nil {
		// Pre-allocate entries and populate it
		scratch.Clear()
		// IPS & GROUPS HERE
		//for word, stats := range m.wordFrequency {
		for _, entry := range topList {
			word := entry.key
			stats := m.wordFrequency[word]
			// topList is derived from wordFrequency during this call, so stats
			// should still be present unless the selection source changes.

			// list ordering is based on peakWordEntry.countPlusFirst order
			countPlusFirst := stats.primeFlux
			// ips are low countPlusFirst entities, augment countPlusFirst with burstiness and delta
			// so THE most interesting are at the top
			// ipScratch specific stuff start
			if countPlusFirst > 8 {
				// scale current + last by these? still not quite right
				// delta is also part of burstiness but its getting double represented anyway
				countPlusFirst += int(stats.burstiness()*100) + int(stats.agentDeltaMetric*10)
			}
			// ShouldConsider() logic pulled out and inlined here
			if len(h) < limit || countPlusFirst > h[0].count {
				// Eligibility is checked before insertion so the heap helper can stay minimal.
				m.topTracker.MaybeForSureAdd(wordEntry{
					word:       word,
					count:      countPlusFirst,
					stat:       stats,
					hasCapture: stats.source.captureColor != "",
				})
			}
		}
	} else {
		// WORDS & REFS GO HERE
		// same as above but without the ipScratch and ip group computation
		//for word, stats := range m.wordFrequency {
		for _, entry := range topList {
			word := entry.key
			stats := m.wordFrequency[word]
			// topList is derived from wordFrequency during this call, so stats
			// should still be present unless the selection source changes.
			countPlusFirst := stats.primeFlux
			// ShouldConsider() logic pulled out and inlined here
			if len(h) < limit || countPlusFirst > h[0].count {
				// Eligibility is checked before insertion so the heap helper can stay minimal.
				m.topTracker.MaybeForSureAdd(wordEntry{
					word:       word,
					count:      countPlusFirst,
					stat:       stats,
					hasCapture: stats.source.captureColor != "",
				})
			}

		}
	}
	// Cut the list of thousands of entries to ~100
	entries := m.topTracker.Top()
	return entries
}

func (m *InterestingWordMatcher) writeMatchedEntry(entry *wordEntry, selectedMatcherColor, baseColor, secondaryInfo string) string {
	stat := entry.stat

	// Base colors
	color := baseColor
	valueColor := baseColor

	// Override with captureColor if available
	if c := stat.captureColor(); c != "" {
		color = c
	}

	// Apply selection styling
	highlight := ""
	color, valueColor, highlight = m.setSelectionColors(entry, color, valueColor, "", selectedMatcherColor)

	// Apply error styling
	highlight, color = m.setErrorColors(entry, highlight, color)

	// Final color tag with highlight, if needed
	if highlight != "" {
		color = color + ":" + highlight
	}

	return fmt.Sprintf(m.fullFormat,
		color,
		stat.allHistoryIndicator(),
		entry.word,
		valueColor,
		secondaryInfo,
	)
}

func (m *InterestingWordMatcher) setErrorColors(entry *wordEntry, newHighlight string, color string) (string, string) {
	if entry.stat.lastStatus[0] == '4' || entry.stat.lastStatus[0] == '5' {
		if m.selected() && entry.word == m.selectionKey() {
			newHighlight = PattyErrorSelection
			color = "white"
		} else {
			newHighlight = PattyErrorHighlight
			color = "black"
		}
		if isLowishHistory(color) {
			color = "black"
		}
	}
	return newHighlight, color
}

func (m *InterestingWordMatcher) setSelectionColors(entry *wordEntry, color string, valueColor string, newHighlight string, selectedMatcherColor string) (string, string, string) {
	if m.selected() && (entry.word == m.selectionKey() || (entry.key != "" && entry.key == m.selectionKey())) {
		if isLowHistory(color) {
			color = "black"
		}
		if isLowHistory(valueColor) {
			valueColor = "black"
		}
		newHighlight = PattyHighlight
	} else if selectedMatcherColor != "" && selectedMatcherColor == color {
		newHighlight = PattySecondaryHighlight
	}
	return color, valueColor, newHighlight
}

func (m *InterestingWordMatcher) selectionKey() string {
	return m.selectedKey
}

func (m *InterestingWordMatcher) selectDisplayItem(selectionIndex int) {
	m.selectedGraphCache = ""
	oldState := PattyGraph.selectedInterestingMatcher
	if m != oldState && oldState != nil {
		oldState.selectedKey = ""
	}

	newKey := ""
	if selectionIndex >= 0 && selectionIndex < len(m.currentListing) {
		newKey = m.currentListing[selectionIndex]
	}
	if m == oldState && m.selectedKey == newKey {
		m.selectedKey = ""
	} else {
		m.selectedKey = newKey
	}

	if m.selectedKey == "" {
		PattyGraph.selectedInterestingMatcher = nil
	} else {
		PattyGraph.selectedInterestingMatcher = m
		if PattyGraph.showTicker {
			m.pushSelectionStats()
		}
	}
}

func (m *InterestingWordMatcher) pushSelectionStats() {
	stats := m.selectedStats()
	if stats == nil {
		return
	}
	peak := stats.count
	bottom := stats.count
	sum := 0
	historyLength := stats.historyLength()
	avg := 0.0
	if historyLength > 0 {
		for _, count := range stats.historySlice() {
			if count > peak {
				peak = count
			}
			if count < bottom {
				bottom = count
			}
			sum += count
		}
		avg = float64(sum) / float64(historyLength)
	}
	color := "[white]"
	if captureColor := stats.captureColor(); captureColor != "" {
		color = "[" + captureColor + "]"
	}
	pushPrintNow(fmt.Sprintf("%s %s%s[default]: ▲%s ▼%s ≈%.0f",
		m.selectionLabel(),
		color,
		m.selectedKey,
		trimmedCounts(peak),
		trimmedCounts(bottom),
		avg,
	))
}

func (m *InterestingWordMatcher) selectionLabel() string {
	switch m {
	case PattyGraph.wordsMatcher:
		return "word"
	case PattyGraph.refsMatcher:
		return "ref"
	case PattyGraph.ipsMatcher:
		return "ip"
	default:
		return m.mName
	}
}

func (m *InterestingWordMatcher) selectedStats() *WordStats {
	if m == nil || m.selectedKey == "" {
		return nil
	}
	stats := m.wordFrequency[m.selectedKey]
	if stats == nil && m.ipScratch != nil {
		stats = m.ipScratch.prefixStats[m.selectedKey]
	}
	return stats
}

func (m *InterestingWordMatcher) selectDisplayItemByKey(selection string) (int, bool) {
	if m == nil {
		return -1, false
	}
	if len(m.currentListing) == 0 {
		m.renderSparklineRow()
	}
	if len(m.currentListing) == 0 {
		return -1, false
	}
	needle := strings.TrimSpace(selection)
	if needle == "" {
		m.selectDisplayItem(-1)
		return -1, true
	}
	exactIdx := -1
	partialIdx := -1
	for i, key := range m.currentListing {
		if key == needle {
			exactIdx = i
			break
		}
		if partialIdx == -1 && strings.Contains(key, needle) {
			partialIdx = i
		}
	}
	idx := exactIdx
	if idx == -1 {
		idx = partialIdx
	}
	if idx == -1 {
		return -1, false
	}
	m.selectDisplayItem(idx)
	return idx, true
}

// secondaryEntryMetric is the tab-cycled info displayed on the right side of every entry.
func (m *InterestingWordMatcher) secondaryEntryMetric(stats *WordStats, nDenom float64) string {
	switch PattyGraph.secondaryView {
	case SecondaryViewPattyFactor:
		if m.ipScratch != nil {
			fullStat := stats.burstiness()
			//secondaryKeyBuilder.WriteString(fmt.Sprintf("%6.2f", fullStat))
			return fmt.Sprintf("%6.2f", fullStat)
		} else {
			entryStatNormalized := stats.normalized()
			return fmt.Sprintf("%6.2f", entryStatNormalized/nDenom)
		}
	case SecondaryViewPrimeFlux:
		fullStat := stats.primeFlux
		return fmt.Sprintf("%6d", fullStat)
	case SecondaryViewHistoryDepth:
		fullStat := stats.historyLength()
		return fmt.Sprintf("  [%d]", fullStat)
	case SecondaryViewAgentDelta:
		return fmt.Sprintf("%5d%%", int(stats.agentDeltaMetric*100))
	case SecondaryViewSparkline:
		return fmt.Sprintf("%-6s", m.miniSparkForStats(stats))
	case SecondaryViewBytes:
		return fmt.Sprintf("%6s", formatBytes64(stats.bytes))
	default:
		return ""
	}
}

func (m *InterestingWordMatcher) miniSparkForStats(stats *WordStats) string { // EXPENSIVE
	return miniReverseSparklineFromArray(stats.reversedHistorySlice()) // EXPENSIVE
}

func isInteresting(word string) bool {
	// Skip words shorter than minWordLength or those in the commonWords set
	if len(word) < DefaultMinWordLength || commonWords[word] {
		return false
	}
	// Check if the word is entirely numeric
	isNumeric := true
	for _, ch := range word {
		if ch != '.' && ch != 'E' && ch != '_' && (ch < '0' || ch > '9') {
			isNumeric = false
			break
		}
	}
	if isNumeric {
		return false
	}

	return true
}

func (m *InterestingWordMatcher) migrateIps() {
	localMax := ""
	lastCount := 0
	for word, stats := range m.wordFrequency {
		if stats.historyLength() > 5 && stats.count > lastCount && stats.source.captureColor == "" {
			lastCount = stats.count
			localMax = word
		}
	}

	// Pretty sure interval lines comparison masks the lastMonitorMax comparison
	//if lastCount == 0 || lastCount < (lastMonitorMax+lastLastMonitorMax)/2 || lastCount < PattyGraph.intervalLines/10 {
	if lastCount == 0 || float64(lastCount) < lastMonitorMaxBuf.nFluxAvg(fluxDepth) || lastCount < PattyGraph.intervalLines/10 {
		return
	}

	for _, matcher := range PattyGraph.matchers {
		if matcher.matcherName() == localMax {
			return
		}
	}

	topIpMatcher := StartsWithMatcher(localMax, []string{localMax})
	topIpMatcher.intervalCount = lastCount
	topIpMatcher.history = m.wordFrequency[localMax].historySlice()

	if placeMatcher(topIpMatcher, matcherFirst) {
		botsMigrated++
	}
	//m.wordFrequency[localMax].spawned = true
}

// purgePeakWords If isPeak gets used, this will be more work than dropping the data
func (m *InterestingWordMatcher) purgePeakWords() {
	m.peakWords = []string{}
	m.peakWordsSet = make(map[string]bool, peakWordLimit)
	m.peakEmptyIntervals = make(map[string]int, peakWordLimit)
	m.peakContentionCount = 0
	m.peakContentionActive = false
	m.peakRetiredCount = 0
	m.peakRetirementGrace = 0
}

//	func (m *InterestingWordMatcher) purgeHistory() {
//		m.wordFrequency = make(map[string]*WordStats, 1000)
//	}

func (m *InterestingWordMatcher) asMatcher() *Matcher {
	return nil
}

// Instead of subclassing for the simple cases, inject behavior function pointers for specific behavior overrides
func WordMatcherFactory(matcherType string) *InterestingWordMatcher {
	wordsPurgeInterval, refsPurgeInterval, ipsPurgeInterval := lookupPurgeIntervals(pattyPushFactor)
	switch matcherType {
	case "words":
		word := NewInterestingWordMatcher(matcherType, wordsPurgeInterval)
		word.lineParser = wordsParseLine
		word.lineTokenizer = tokensForWords
		return word
	case "refs":
		refs := NewInterestingWordMatcher(matcherType, refsPurgeInterval)
		refs.lineParser = refsParseLineFast
		refs.lineTokenizer = tokensForRefs
		return refs
	case "ips":
		ips := NewInterestingWordMatcher(matcherType, ipsPurgeInterval)
		ips.lineParser = ipsParseLine
		ips.lineTokenizer = tokensForIps
		ips.displayWidth -= 3
		ips.titleFormat = fmt.Sprintf("%%-%ds", ips.displayWidth-19)
		ips.fullTitleFormat = "[#F4F4F4]Interesting " + ips.titleFormat + "[#999999]%6.6s[default:-]\n"

		ips.groupTracker = NewTopPrefixTracker(10)
		ips.ensureFullFormat()
		ips.ipScratch = createIpScratch()
		return ips
	default:
		log.Fatalf("Unknown wordMatcher type: %s", matcherType)
	}
	return nil
}

func createIpScratch() *ipGroupScratch {
	const startingMapSize = 4096
	const groupMapSize = 32
	return &ipGroupScratch{
		prefixCounts:          make(map[string]int, startingMapSize),
		prefixFirstPlusCounts: make(map[string]int, startingMapSize),

		mEntries:       make(map[string]int, groupMapSize),
		mColors:        make(map[string]string, groupMapSize),
		mMatchers:      make(map[string]string, groupMapSize),
		prefixColors:   make(map[string]string, groupMapSize),
		prefixMatchers: make(map[string]string, groupMapSize),
		prefixDepths:   make(map[string]int, groupMapSize),
		prefixBursts:   make(map[string]float64, groupMapSize),
		prefixBytes:    make(map[string]uint64, groupMapSize),
		prefixDeltas:   make(map[string]float64, groupMapSize),
		prefixMembers:  make(map[string]int, groupMapSize),
		//prefixHistoryBufs:        make(map[string]*ringBuffer, groupMapSize),
		prefixHistorAggregateBufs: make(map[string]*ringSeriesAccumulator, groupMapSize),
		prefixStats:               make(map[string]*WordStats, groupMapSize),
		prefixFirstLines:          make(map[string]string, groupMapSize),
		prefixLastLines:           make(map[string]string, groupMapSize),
		prefixFirstIntervalLines:  make(map[string]string, groupMapSize),

		prefixToIPs:        make(map[string]map[string]struct{}, groupMapSize),
		activePrefixes:     make(map[string]struct{}, groupMapSize),
		activePrefixCounts: make(map[string]int, startingMapSize),
	}
}

// should rename to something about periodic maintenance.
func (m *InterestingWordMatcher) compactFrequencyMap() {
	newLength := len(m.wordFrequency)
	fresh := make(map[string]*WordStats, newLength)
	for key, val := range m.wordFrequency {
		fresh[key] = val
	}
	m.wordFrequency = fresh

	m.lruTracker.EvictBottomPercent(10)
}

// shared formatting for all word entries, must be readjusted if displayWidth changes
func (m *InterestingWordMatcher) ensureFullFormat() {
	keyFormat := fmt.Sprintf("%%-%d.%ds", m.displayWidth-8, m.displayWidth-8)
	m.fullFormat = "[%s]%s" + keyFormat + "[%s]%6.6s[-:-]\n"
}

// Click value selection in the spark graph area.
func (m *InterestingWordMatcher) selectedHistoryAt(idx int) int {
	key := m.selectedKey
	if key != "" {
		if stats, exists := m.wordFrequency[key]; exists {
			if idx < stats.historyLength() {
				return stats.historyAt(stats.historyLength() - idx - 1)
			}
			//} else if h, exists2 := m.ipScratch.prefixHistoryBufs[key]; exists2 { // IP Groups aggregated selection
			//	if idx < h.Len() {
			//		return h.At(h.Len() - 1 - idx)
			//	}
		} else if h, exists2 := m.ipScratch.prefixHistorAggregateBufs[key]; exists2 { // IP Groups aggregated selection
			if idx < h.Len() {
				return h.unsafeAt(h.Len() - 1 - idx)
			}
		}
	}
	return 0
}

type prefixCount struct {
	prefix         string
	countPlusFirst int
	count          int
}

// Uses heap calls and interfaces, but its not nearly as critical as TopWordTracker, so its ok.
func NewTopPrefixTracker(maxSize int) *TopPrefixTracker {
	return &TopPrefixTracker{
		limit: maxSize,
	}
}

// prefixGroup specalization to avoid interface overhead
type TopPrefixTracker struct {
	limit int
	h     prefixCountHeap
	buf   []prefixCount
}

type prefixCountHeap []prefixCount

func (h prefixCountHeap) Len() int           { return len(h) }
func (h prefixCountHeap) Less(i, j int) bool { return h[i].countPlusFirst < h[j].countPlusFirst }
func (h prefixCountHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *prefixCountHeap) Push(x interface{}) {
	*h = append(*h, x.(prefixCount))
}

func (h *prefixCountHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

func (t *TopPrefixTracker) ShouldConsider(newCount int) bool {
	return len(t.h) < t.limit || newCount > t.h[0].countPlusFirst
}
func (t *TopPrefixTracker) MaybeAdd(entry prefixCount) {
	if len(t.h) < t.limit {
		heap.Push(&t.h, entry)
	} else if entry.countPlusFirst > t.h[0].countPlusFirst {
		heap.Pop(&t.h)
		heap.Push(&t.h, entry)
	}
}

func (t *TopPrefixTracker) Reset() {
	t.h = t.h[:0]
}

func (t *TopPrefixTracker) Top() []prefixCount {
	if cap(t.buf) < len(t.h) {
		t.buf = make([]prefixCount, 0, len(t.h))
	}
	result := t.buf[:0]
	result = append(result, t.h...)
	return result
}

// Raw, no heap library, no interface, all inlined go heap code
// part of the hottest hot path
type TopWordTracker struct {
	limit int
	h     []wordEntry // length ≤ limit, capacity == limit
}

func NewTopWordTracker(limit int) *TopWordTracker {
	return &TopWordTracker{
		h:     make([]wordEntry, 0, limit),
		limit: limit,
	}
}

func (t *TopWordTracker) Reset() {
	t.h = t.h[:0]
}

func (t *TopWordTracker) Top() []wordEntry {
	return t.h
}

// Manual push with heap invariant (min-heap). Very hot path
// This assumes the caller has pre-met one of the two conditions:
//
//	1 Buffer is still filling up
//	2 new entry for sure should be added bc the new entry has more counts
func (t *TopWordTracker) MaybeForSureAdd(entry wordEntry) {
	if len(t.h) < t.limit {
		// Insert and percolate up
		t.h = t.h[:len(t.h)+1] // manually grow length
		t.up(len(t.h)-1, entry)
	} else { //} if entry.count > t.h[0].count {
		// Replace root and percolate down
		t.down(0, entry)
	}
}

//func (t *TopWordTracker) MaybeAdd(entry wordEntry) {
//	if len(t.h) < t.limit {
//		// Insert and percolate up
//		t.h = t.h[:len(t.h)+1] // manually grow length
//		t.up(len(t.h)-1, entry)
//	} else if entry.count > t.h[0].count {
//		// Replace root and percolate down
//		t.down(0, entry)
//	}
//}

func (t *TopWordTracker) up(pos int, entry wordEntry) {
	h := t.h
	for pos > 0 {
		parent := (pos - 1) / 2
		if entry.count >= h[parent].count {
			break
		}
		h[pos] = h[parent]
		pos = parent
	}
	h[pos] = entry
}

func (t *TopWordTracker) down(pos int, entry wordEntry) {
	h := t.h
	n := len(h)
	for {
		left := 2*pos + 1
		if left >= n {
			break
		}
		smallest := left
		right := left + 1
		if right < n && h[right].count < h[left].count {
			smallest = right
		}
		if h[smallest].count >= entry.count {
			break
		}
		h[pos] = h[smallest]
		pos = smallest
	}
	h[pos] = entry
}

/////// new lru for words ///////

type ScoredEntry struct {
	key   string
	score int
}

type ScoredLRUTracker struct {
	limit int
	index map[string]*list.Element
	list  *list.List // Front = most recently seen, Back = least
}

func NewScoredLRUTracker(limit int) *ScoredLRUTracker {
	return &ScoredLRUTracker{
		limit: limit,
		index: make(map[string]*list.Element, limit),
		list:  list.New(),
	}
}
func (t *ScoredLRUTracker) MarkSeen(key string, score int) {
	if elem, ok := t.index[key]; ok {
		entry := elem.Value.(*ScoredEntry)
		entry.score = score
		t.list.MoveToFront(elem)
		return
	}

	// If full, check if the new score is worth tracking
	if t.list.Len() >= t.limit {
		back := t.list.Back()
		if back != nil {
			worst := back.Value.(*ScoredEntry)
			if score <= worst.score {
				return // New item isn't better than the worst one; skip
			}
			delete(t.index, worst.key)
			t.list.Remove(back)
		}
	}

	// Insert the new entry
	entry := &ScoredEntry{key: key, score: score}
	elem := t.list.PushFront(entry)
	t.index[key] = elem
}

func (t *ScoredLRUTracker) Delete(key string) {
	if elem, ok := t.index[key]; ok {
		t.list.Remove(elem)
		delete(t.index, key)
	}
}

func (t *ScoredLRUTracker) TopN(n int) []ScoredEntry {
	result := make([]ScoredEntry, 0, n)
	count := 0
	for e := t.list.Front(); e != nil && count < n; e = e.Next() {
		result = append(result, *(e.Value.(*ScoredEntry)))
		count++
	}
	return result
}

func (t *ScoredLRUTracker) EvictBelow(minScore int) {
	for e := t.list.Back(); e != nil; {
		prev := e.Prev()
		entry := e.Value.(*ScoredEntry)
		if entry.score >= minScore {
			break
		}
		t.list.Remove(e)
		delete(t.index, entry.key)
		e = prev
	}
}
func (t *ScoredLRUTracker) EvictBottomPercent(pct float64) {
	if pct <= 0 || pct >= 1 {
		return // Invalid percent
	}
	evictCount := int(float64(t.list.Len()) * pct)
	if evictCount < 1 {
		return
	}

	// Traverse from back (least recently seen)
	for i := 0; i < evictCount; i++ {
		elem := t.list.Back()
		if elem == nil {
			break
		}
		entry := elem.Value.(*ScoredEntry)
		delete(t.index, entry.key)
		t.list.Remove(elem)
	}
}

const IpGroupActiveThreshold = 15

func (s *ipGroupScratch) Add(ip, prefix string) {
	ipSet, exists := s.prefixToIPs[prefix]
	if !exists {
		ipSet = make(map[string]struct{}, 10)
		s.prefixToIPs[prefix] = ipSet
	}
	if _, seen := ipSet[ip]; !seen {
		ipSet[ip] = struct{}{}
		// bump the count
		newCount := s.activePrefixCounts[prefix] + 1
		s.activePrefixCounts[prefix] = newCount
		// if we've just hit the threshold, mark active
		if newCount == IpGroupActiveThreshold {
			s.activePrefixes[prefix] = struct{}{}
		}
	}
}

func (s *ipGroupScratch) Add_old(ip string, prefix string) {
	ipSet, exists := s.prefixToIPs[prefix]
	if !exists {
		ipSet = make(map[string]struct{}, 10)
		s.prefixToIPs[prefix] = ipSet
	}
	ipSet[ip] = struct{}{}
}
func (s *ipGroupScratch) Remove(ip, prefix string) {
	minSize := 15
	if ipSet, ok := s.prefixToIPs[prefix]; ok {
		if _, seen := ipSet[ip]; seen {
			delete(ipSet, ip)
			newCount := s.activePrefixCounts[prefix] - 1
			if newCount <= 0 {
				// no members → forget it entirely
				delete(s.prefixToIPs, prefix)
				delete(s.activePrefixCounts, prefix)
				delete(s.activePrefixes, prefix)
			} else {
				s.activePrefixCounts[prefix] = newCount
				// if we've fallen below the threshold, de-activate
				if newCount == minSize-1 {
					delete(s.activePrefixes, prefix)
				}
			}
		}
	}
}

// MUST be called when m.wordFrequency[word] is removed
func (s *ipGroupScratch) Remove_old(ip string, prefix string) {
	if ipSet, ok := s.prefixToIPs[prefix]; ok {
		delete(ipSet, ip)
		if len(ipSet) == 0 {
			delete(s.prefixToIPs, prefix)
		}
	}
}

// Naive "single" threaded model lets this optimization be so simple minded
var activePrefixScratch = make([]string, 0, 20)

func (s *ipGroupScratch) ActivePrefixes() []string {
	// we know this slice is only as big as the last ActivePrefixes call
	if cap(activePrefixScratch) < len(s.activePrefixes) {
		activePrefixScratch = make([]string, 0, len(s.activePrefixes))
	}
	result := activePrefixScratch[:0]

	for prefix := range s.activePrefixes {
		result = append(result, prefix)
	}
	s.activePrefixesCountMetric = len(result)
	return result
}

//func (s *ipGroupScratch) ActivePrefixes(IpGroupActiveThreshold int) []string {
//	//result := make([]string, 0, len(s.prefixToIPs))
//	if cap(activePrefixScratch) < len(s.prefixToIPs) {
//		activePrefixScratch = make([]string, 0, len(s.prefixToIPs)*2)
//	}
//	result := activePrefixScratch[:0]
//
//	for prefix, ipSet := range s.prefixToIPs {
//		if len(ipSet) >= IpGroupActiveThreshold {
//			result = append(result, prefix)
//		}
//	}
//	return result
//}

var activeIpSetScratch = make([]string, 0, 20)

func (s *ipGroupScratch) ActivePrefixMembers(prefix string) []string {
	ipSet, ok := s.prefixToIPs[prefix]
	if !ok {
		return nil
	}
	if cap(activeIpSetScratch) < len(ipSet) {
		activeIpSetScratch = make([]string, 0, len(ipSet)*2)
	}
	ips := activeIpSetScratch[:0]
	for ip := range ipSet {
		ips = append(ips, ip)
	}
	return ips
}
func (m *InterestingWordMatcher) updatePeakWordStats() {
	if PattyGraph.intervalsCompleted < pattyGracePeriod {
		return
	}
	count := len(m.peakWords)
	m.peakWordCounts = append(m.peakWordCounts, count)
	m.totalPeakCounts += count

	if len(m.peakWordCounts) > 80 {
		removed := m.peakWordCounts[0]
		m.peakWordCounts = m.peakWordCounts[1:]
		m.totalPeakCounts -= removed
	}
}
func (m *InterestingWordMatcher) averagePeakWords() float64 {
	if len(m.peakWordCounts) == 0 {
		return 0
	}
	return float64(m.totalPeakCounts) / float64(len(m.peakWordCounts))
}
