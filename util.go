// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"container/list"
	"errors"
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	PattyGraphAuthor    = "Jasen Minton"
	PattyGraphGithubUrl = "https://github.com/jwminton/pattyGraph"
	PattyGraphName      = "PattyGraph"
	// NOTE: PattyGraphVersion is used by compile.sh for repackaging labeling
	PattyGraphVersion = "0.1.7-dev"
)

const (
	PattyOrange             = "#FFA96F"
	PattyBotsColor          = "[#A0FFFF]"
	PattyDisabledBotsColor  = "[#507070]"
	PattyLinesColor         = "[beige]"
	PattyBytesColor         = "[ivory]"
	PattyErrorColor         = "[#E8CFE3]"
	PattyHighlight          = "#4C4C4C"
	PattySecondaryHighlight = "#202020"
	PattyErrorSelection     = "#804060"
	PattyErrorHighlight     = "#A07090"
)

const (
	PattyPrintWidth         = 100
	progressBarWidth        = 15
	DefaultHistoryDepth     = 80
	ExpertScaleWidth        = 43
	pattyPushFactorDefault  = 5
	pattyScaleFactorDefault = 1.0
	pattyGracePeriodDefault = 15
	defaultLogFilename      = "./access.log"
	DefaultIntervalSize     = 60
	DefaultMinWordLength    = 3
	pattyBurstMax           = 4.0
	pattyBurstStep          = 0.1
	pattyBurstMin           = 0.1
	DefaultMBToRead         = 50
	DefaultFluxDepth        = 3
)

const (
	upArrow    = "↑"
	levelArrow = ":"
	downArrow  = "↓"

	botsDisplayWidth = 26
	InlinePreamble   = "!!!"
)

// AutobotColors is ordered to provide a long sequence of visibly distinct row
// colors, especially when more than one PattyGraph session is open.
var AutobotColors = []string{
	// Reordered Colors
	"[fuchsia]", "[yellow]", "[chartreuse]", "[tomato]",
	"[deepskyblue]", "[moccasin]", "[mediumvioletred]",
	"[darkviolet]", "[orangered]", "[green]", "[mediumturquoise]",
	"[dodgerblue]", "[palegreen]", //"[lavender]",
	"[coral]", "[plum]", "[cadetblue]", "[forestgreen]",
	"[khaki]", "[slateblue]", "[turquoise]", "[limegreen]", "[gold]",
	"[indigo]", "[saddlebrown]", "[rosybrown]", "[cornflowerblue]",
	"[orange]", "[seagreen]", "[peachpuff]", "[darkturquoise]",
	"[lime]", "[lemonchiffon]", "[springgreen]", "[darkorange]",
	"[lightseagreen]", "[mediumseagreen]", "[lightcoral]", "[darkcyan]",
	"[orchid]", "[lightsteelblue]", "[goldenrod]", "[sandybrown]",
	"[darkseagreen]", "[peru]", "[dimgray]", "[crimson]",
	"[mediumspringgreen]", "[thistle]", "[steelblue]", "[aquamarine]",
	"[violet]", "[firebrick]", "[skyblue]", "[chocolate]",
	"[olive]", "[deeppink]", "[teal]", "[darkslateblue]",
	"[hotpink]", "[mediumslateblue]", "[mediumaquamarine]", "[darkmagenta]",
	"[lightsalmon]", "[mediumorchid]", "[rebeccapurple]", "[darkolivegreen]",
}

// monoColors maps shallow interesting-entry history depth to grayscale display
// colors. Early depths get distinct steps, while later depths compress toward
// white because entries usually become either short-lived or established quickly.
// Some code depends on monoColors being at least 4 elements long.
var monoColors = []string{
	"#363636", //  16% brightness
	"#494949", //  24% brightness
	"#5D5D5D", //  36% brightness
	"#7A7A7A", //  48% brightness
	"#999999", //  60% brightness
	"#BDBDBD", //  74% brightness
	"#D9D9D9", //  85% brightness
	"#FFFFFF", // 100% brightness (pure white)
}
var sparkColorCache []string
var lastGraceUsed int
var displayFreezeCountdown int
var displayMod = 1

// when more than 'peakIpThreshold' ip's are in an octet, form a grouping for the matcher
var peakIpThreshold = 10
var firstColorWins bool

// commonWordList filters stable, high-frequency tokens from InterestingWordMatcher
// streams. These words are useful context inside raw logs, but rarely deserve
// top-list attention during an emergency or forensic pass. Commented entries
// document prior tuning decisions so future changes can see what was tested and
// removed.
var commonWordList = []string{
	"GET", "POST", "PUT", "HEAD", "OPTIONS", "DELETE", "CONNECT", "TRACE", "PATCH", // HTTP methods
	"+http", "http", "+https", "https", "HTTP", "HTTP/1.1", "HTTP/2.0", "HTTPS", // Protocols
	"Mozilla", "Gecko", "Safari", "Chrome", "Firefox", "KHTML", // Browser & Layout indicators
	//	"text", "plain", "html", "xml", "json", // Common content types
	//	"keep-alive", "close", "upgrade-insecure-requests", // HTTP headers
	//	"localhost", "example", "com", "org", "net", // Common domain-related terms
	//	"cache-control", "pragma", "expires", "cookie", "accept", "language", // More HTTP headers
	//	"host", "user-agent", "accept-encoding", "connection", "content-type", "content-length", // Standard headers
	//	"gzip", "deflate", "br", // Encoding types
	//	"image", "png", "jpg", "jpeg", "svg", "gif", // Image formats
	//	"script", "stylesheet", "font", // Asset types
	"en-us", "en-gb", "en-US", "in-id", "id-ID", // common expected langs
	"Android", "Linux", "Mobile", "SAMSUNG", "iPhone", "iPad", "Windows", "Macintosh", "Mac", "Intel", "Edg", "OPR", // platforms/archs
	"Win64", "x64", "x86_64", "X11", "iOS", "EdgiOS", "Darwin", "CrOS", "CPU", "GSA", "Ddg", "CriOS",
	"like", "Version", "Build", "Nexus", "AppleWebKit", "https:", "compatible", // User-Agent words to ignore and misc terms
}

var browsPattern = `(Chrome|CriOS|Firefox|FxiOS|Safari|DuckDuckGo|` +
	`Edg|Edge|OPR|FxiOS|MSIE|Trident|Brave|PlayStation|Vivaldi|Baidu|SeaMonkey|Maxthon|Puffin|` +
	`Silk|Sogou|Dolfin|IceCat|Iceweasel|Waterfox|` + // extra niche platforms
	`K-Meleon|PaleMoon|Avant|Epiphany|` + // extra niche platforms
	`\w{2,} ?[Bb]rowser)+`

// Original capture pattern. Too historical to just drop from code.
// Much of original bot detection was based on this
//var ipCapturePattern = `^"([^"]*?)" (\d+) (\d+) "([^"]*?)" "(.*?)"$`

func pattyMonoColorForInt(eLen int) string {
	if sparkColorCache == nil || lastGraceUsed != pattyGracePeriod {
		fillMonoColorCache()
	}
	if eLen <= 0 {
		return monoColors[0]
	}
	if eLen >= pattyGracePeriod {
		return sparkColorCache[pattyGracePeriod]
	}
	return sparkColorCache[eLen]
}
func fillMonoColorCache() {
	monoColorsSize := len(monoColors) - 1
	lastGraceUsed = pattyGracePeriod
	sparkColorCache = make([]string, DefaultHistoryDepth+1)

	for eLen := 0; eLen <= DefaultHistoryDepth; eLen++ {
		var color string
		if eLen <= 1 {
			color = monoColors[0]
		} else if eLen == 2 {
			color = monoColors[1]
		} else if eLen >= pattyGracePeriod {
			color = monoColors[monoColorsSize]
		} else {
			normalizedDepth := float64(eLen) / float64(pattyGracePeriod+1)
			scaledDepth := math.Pow(normalizedDepth, 0.5)
			index := int(scaledDepth * float64(monoColorsSize))
			if index > monoColorsSize {
				index = monoColorsSize
			} else if index < 2 {
				index = 2
			}
			color = monoColors[index]
		}
		sparkColorCache[eLen] = color
	}
}

func areLikelyIPPatterns(patterns []string) bool {
	for _, p := range patterns {
		if strings.Count(p, ".") < 2 {
			return false
		}
		if !unicode.IsDigit(rune(p[0])) {
			return false
		}
	}
	return true
}

// For highlighting to know when to override low mono colors
func isLowHistory(color string) bool {
	return color == monoColors[0] || color == monoColors[1]
}

// For error highlighting to know when to override low-ish mono colors
func isLowishHistory(color string) bool {
	return isLowHistory(color) || color == monoColors[2] || color == monoColors[3]
}

// This is where more high level matchers were going to be injected. Browser detection, specialURL detection, etc.
func namedMatchers() []*Matcher {
	newM := MatcherFactory(BotsMatcherName)
	return []*Matcher{newM}
}

// Define the sparkline characters, from low to high values
var sparks = []string{" ", "▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}

const gammaConst = 0.5
const gammaTop = 1000.0

// values needlessly pulled out and math in odd steps to get it right first
func sparklineFromArray(minIn int, maxIn int, history []int) string {
	// Generate sparkline string by mapping values to sparkline characters
	var sparkline strings.Builder
	for i := len(history) - 1; i >= 0; i-- { // Start from the most recent value
		val := history[i]
		// Scale val to fit into the sparkline range
		level := 0
		span := maxIn - minIn
		index := val - minIn

		if span < len(sparks)-1 {
			span = len(sparks) - 1
			maxIn = minIn + span
		}
		// map minIn ∈ [0, 5000] → t ∈ [0,1]
		t := float64(minIn) / gammaTop
		if t < 0 {
			t = 0
		}
		if t > 1 {
			t = 1
		}
		// gamma interpolates from 0.5 to 1.0 as minIn goes toward 1000
		gamma := 0.5 + gammaConst*t

		if index <= 0 {
			level = 0
		} else if val >= maxIn {
			level = len(sparks) - 1
		} else {
			// Normalize the value to [0, 1]
			normalizedVal := float64(val-minIn) / float64(maxIn-minIn)

			// Apply quad scaling
			//scaledVal := math.Sqrt(normalizedVal)
			//scaledVal := math.Log2(1 + normalizedVal*float64(len(sparks)-1)) // Logarithmic alternative // NO
			scaledVal := math.Pow(normalizedVal, gamma)

			// Map to sparkline levels
			level = int(scaledVal * float64(len(sparks)-1))

			// bounds checking for safety
			if level <= 0 {
				level = 0
			} else if level >= len(sparks) {
				level = len(sparks) - 1
			}
		}
		sparkline.WriteString(sparks[level])
	}
	return sparkline.String()
}

var miniSparkBuilderCache = strings.Builder{}

// TODO: This reverse can be unreversed?
func miniReverseSparklineFromArray(history []int) string {
	start := max(0, PattyGraph.miniWindowIndex-1)
	n := len(history)
	if n > start+6 {
		n = start + 6
	}
	if n <= start {
		return ""
	}

	minIn := 0
	maxIn := max(lastMonitorMaxBuf.Latest(), history[start]*11/10)
	t := float64(minIn) / gammaTop
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	// gamma interpolates from 0.5 to 1.0 as minIn goes toward 5000
	gamma := 0.5 + gammaConst*t

	// Generate sparkline string by mapping values to sparkline characters
	//var sparkline strings.Builder
	miniSparkBuilderCache.Reset()
	level := 0
	span := maxIn - minIn
	for i, val := range history[start:] {
		if i == 6 {
			break
		}
		// Scale val to fit into the sparkline range
		index := val - minIn

		if span < len(sparks)-1 {
			span = len(sparks) - 1
			maxIn = minIn + span
		}

		if index <= 0 {
			level = 0
		} else if val >= maxIn {
			level = len(sparks) - 1
		} else {
			// Normalize the value to [0, 1]
			normalizedVal := float64(val-minIn) / float64(maxIn-minIn)

			// Apply quad scaling
			//scaledVal := math.Sqrt(normalizedVal)
			scaledVal := math.Pow(normalizedVal, gamma)
			// scaledVal := math.Log2(1 + normalizedVal*float64(sparklineLevels)) // Logarithmic alternative

			// Map to sparkline levels
			level = int(scaledVal * float64(len(sparks)-1))

			// bounds checking for safety
			if level <= 0 {
				level = 0
			} else if level >= len(sparks) {
				level = len(sparks) - 1
			}
		}
		miniSparkBuilderCache.WriteString(sparks[level])
	}
	return miniSparkBuilderCache.String()
}
func formatUptime(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60

	if h > 0 {
		return fmt.Sprintf("%dh%02dm", h, m)
	} else if m > 0 {
		return fmt.Sprintf("%dm%02ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
func formatShortDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	ms := int(d.Milliseconds()) % 1000

	switch {
	case h > 0:
		return fmt.Sprintf("%dh%02dm", h, m)
	case m > 0:
		return fmt.Sprintf("%dm%02ds", m, s)
	case s > 0:
		return fmt.Sprintf("%ds%03dms", s, ms)
	default:
		return fmt.Sprintf("%dms", ms)
	}
}

func formatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60

	if h > 0 {
		return fmt.Sprintf("%dh%02dm", h, m)
	} else if m > 0 {
		return fmt.Sprintf("%dm%02ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
func formatCountsUint64(value uint64) string {
	const (
		K = 1_000
		M = 1_000_000
		B = 1_000_000_000
		T = 1_000_000_000_000
	)

	switch {
	case value < K:
		return fmt.Sprintf("%d", value)
	case value < M:
		return fmt.Sprintf("%dK", value/K)
	case value < 10*M:
		return fmt.Sprintf("%.1fM", float64(value)/M)
	case value < B:
		return fmt.Sprintf("%dM", value/M)
	case value < 10*B:
		return fmt.Sprintf("%.1fB", float64(value)/B)
	case value < T:
		return fmt.Sprintf("%dB", value/B)
	case value < 10*T:
		return fmt.Sprintf("%.1fT", float64(value)/T)
	default:
		return fmt.Sprintf("%dT+", value/T)
	}
}

func trimmedCounts(value int) string {
	return strings.TrimSpace(formatCounts(value))
}

func formatCounts(value int) string {
	// Handle values below 10,000
	if value <= 9999 {
		return fmt.Sprintf("%4d", value) // Right-align numbers below 10,000
	}

	// Handle values in the thousands range
	if value <= 999999 {
		thousands := value / 1000
		return fmt.Sprintf("%3dK", thousands) // Format as " XXXK"
	}

	// Handle values in the millions range
	millions := float64(value) / 1000000.0
	if millions < 10.0 { // Only valid for up to 9.9M
		return fmt.Sprintf("%4.1fM", millions) // Format as " X.YM"
	}

	// Fail for values that can't fit in 4 chars
	return "----"
}
func formatBytes64(u uint64) string {
	const (
		KB = 1 << 10
		MB = 1 << 20
		GB = 1 << 30
		TB = 1 << 40
		PB = 1 << 50
		EB = 1 << 60
	)

	switch {
	case u >= EB:
		return fmt.Sprintf("%.2f E", float64(u)/float64(EB))
	case u >= PB:
		return fmt.Sprintf("%2.0fP", float64(u)/float64(PB))
	case u >= TB:
		return fmt.Sprintf("%2.0fT", float64(u)/float64(TB))
	case u >= GB:
		return fmt.Sprintf("%2.0fG", float64(u)/float64(GB))
	case u >= MB:
		return fmt.Sprintf("%2.0fM", float64(u)/float64(MB))
	case u >= KB:
		return fmt.Sprintf("%2.0fK", float64(u)/float64(KB))
	default:
		return fmt.Sprintf("%2dB", u)
	}
}

func formatBytes(bytes int) string {
	const (
		KB = 1 << 10 // 1024 bytes
		MB = 1 << 20 // 1024 KB
		GB = 1 << 30 // 1024 MB
		TB = 1 << 40 // 1024 GB
		PB = 1 << 50 // 1024 TB
		EB = 1 << 60 // 1024 PB
	)

	switch {
	case bytes >= EB:
		return fmt.Sprintf("%.2f E", float64(bytes)/EB)
	case bytes >= PB:
		return fmt.Sprintf("%2.0fP", float64(bytes)/PB)
	case bytes >= TB:
		return fmt.Sprintf("%2.0fT", float64(bytes)/TB)
	case bytes >= GB:
		return fmt.Sprintf("%2.0fG", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%2.0fM", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%2.0fK", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%2dB", bytes)
	}
}

var matcherColorMap = make(map[string]string)

func reassignMatcherColor(name string, color string) {
	cleanName := strings.Trim(name, "*-+")
	newColor := color
	if strings.ToLower(color) == "[auto]" {
		colorIndex = (colorIndex + 1) % len(AutobotColors)
		newColor = AutobotColors[colorIndex]
		matcherColorMap[cleanName] = newColor
	} else {
		matcherColorMap[cleanName] = newColor
	}

	for _, mf := range PattyGraph.matchers {
		m := mf.asMatcher()
		if m != nil && m.name == cleanName {
			m.color = newColor
			m.isColorUserAssigned = true
		}
	}
}

func colorAlreadyUsed(color string) bool {
	for _, v := range matcherColorMap {
		if v == color {
			return true
		}
	}
	return false
}

func setMatcherColor(m *Matcher) {
	if color, exists := matcherColorMap[m.name]; exists {
		m.setColor(color)
		return
	}
	var newColor string
	for newColor = AutobotColors[colorIndex]; colorAlreadyUsed(newColor); newColor = AutobotColors[colorIndex] {
		colorIndex++
		if colorIndex > len(AutobotColors) {
			newColor = "[default]"
			break
		}
	}
	m.setColor(newColor)
	matcherColorMap[m.name] = newColor
}

func botsMatcherIndex() int {
	// Find the index of botsMatcher
	for i, m := range PattyGraph.matchers {
		if m.asMatcher() == PattyGraph.botsMatcher {
			return i
		}
	}
	return -1 // can't happen
}

// UpdateHistoricFlags re-derives the historical scaling boundary from the current
// Bots position. Rows through Bots share global sparkline scale; concrete rows
// below Bots use local scale. Interesting streams terminate this pass because
// they are MatcherFacade rows without concrete Matcher history semantics.
func UpdateHistoricFlags() {
	PattyGraph.overallMax = -1 // cache invalidator
	//_, PattyGraph.dynamicPeakThreshold, _ = PattyGraph.minAvgMaxHistoryAcrossMatchers()
	botsIndex = botsMatcherIndex()

	if botsIndex == -1 {
		// botsMatcher not found in matchers
		return
	}

	// Rows through Bots share historical scale.
	for i := 0; i <= botsIndex; i++ {
		PattyGraph.matchers[i].asMatcher().isHistorical = true
		PattyGraph.matchers[i].asMatcher().historySparklineCache = ""
	}

	// Concrete rows after Bots use local scale until interesting streams begin.
	for i := botsIndex + 1; i < len(PattyGraph.matchers); i++ {
		if PattyGraph.matchers[i].asMatcher() == nil {
			break
		}
		PattyGraph.matchers[i].asMatcher().isHistorical = false
		PattyGraph.matchers[i].asMatcher().historySparklineCache = ""
	}
}

type matcherPlacement uint8

const (
	matcherFirst matcherPlacement = iota
	matcherBeforeBots
	matcherBeforeLines
)

// placeMatcher is the single structural entry point for dynamically created
// matchers. Callers retain ownership of matcher construction, inherited state,
// and placement policy; this function inserts the completed matcher and then
// restores the ordering-dependent runtime state.
func placeMatcher(newMatcher *Matcher, placement matcherPlacement) bool {
	if PattyGraph == nil || newMatcher == nil {
		return false
	}

	insertAt := -1
	switch placement {
	case matcherFirst:
		insertAt = 0
	case matcherBeforeBots:
		insertAt = matcherFacadeIndex(PattyGraph.botsMatcher)
	case matcherBeforeLines:
		insertAt = matcherFacadeIndex(PattyGraph.linesMatcher)
	default:
		return false
	}
	if insertAt < 0 {
		return false
	}

	PattyGraph.matchers = append(PattyGraph.matchers, nil)
	copy(PattyGraph.matchers[insertAt+1:], PattyGraph.matchers[insertAt:])
	PattyGraph.matchers[insertAt] = newMatcher
	finalizeMatcherPlacement(newMatcher)
	return true
}

func matcherFacadeIndex(target *Matcher) int {
	if target == nil {
		return -1
	}
	for i, matcher := range PattyGraph.matchers {
		if matcher.asMatcher() == target {
			return i
		}
	}
	return -1
}

// finalizeMatcherPlacement makes a new row visible to every ordering consumer
// at once. UpdateHistoricFlags refreshes the Bots boundary, historical scaling,
// and matcher sparkline caches after the slice has reached its final shape.
func finalizeMatcherPlacement(newMatcher *Matcher) {
	setMatcherColor(newMatcher)
	UpdateHistoricFlags()
}

func lookupPurgeIntervals(purgeIntensity int) (int, int, int) {
	if purgeIntensity < 0 {
		purgeIntensity = 0
	} else if purgeIntensity >= 11 {
		purgeIntensity = 11
	}

	// Lookup table: [purgeIntensity][words, refs, ips]
	// Values are in seconds (or cycles if you're measuring by interval countPlusFirst)
	purgeTable := [12][3]int{
		{300, 300, 300}, // 0 - least aggressive
		{240, 300, 300}, // 1
		{180, 240, 300}, // 2
		{120, 180, 240}, // 3
		{90, 120, 180},  // 4
		{60, 90, 120},   // 5
		{30, 60, 90},    // 6
		{20, 30, 60},    // 7
		{10, 20, 30},    // 8
		{5, 10, 20},     // 9
		{1, 5, 10},      // 10
		{1, 1, 1},       // 11 - most aggressive
	}

	row := purgeTable[purgeIntensity]
	return row[0], row[1], row[2]
}

/*
*
Tokenize the user-agent strings and then compute a levenshtein distance utilizing parsed tokens instead of individual
chars
Given the user agents:

UA1: "Mozilla/5.0 (Linux; Android 10) Chrome/116.0.0.0 Mobile Safari/537.36"
UA2: "Mozilla/5.0 (Linux; Android 11) Chrome/117.0.0.0 Mobile Safari/537.36"
Tokenized as:

TokensA: ["Mozilla/5.0", "(Linux;", "Android", "10)", "Chrome/116.0.0.0", "Mobile", "Safari/537.36"]
TokensB: ["Mozilla/5.0", "(Linux;", "Android", "11)", "Chrome/117.0.0.0", "Mobile", "Safari/537.36"]
Computation:
Levenshtein Distance: 2 (substitutions for "10)" → "11)" and "Chrome/116.0.0.0" → "Chrome/117.0.0.0").
Ratio:
distance/lenA=2/7=0.2857.

A ratio near 0.0 → Highly similar user agents.
A ratio near 1.0 → Completely divergent user agents.

0.0: Exact match (e.g. identical user agents).
0.14: Small variation (e.g. version differences or slight platform change).
0.57: Major difference (e.g. different platforms and rendering libaries).
1.0+: Nearly unrelated strings (e.g. bot vs real browsers).

Efficiency is more important than accuracy
*/
//func levenshteinTokensRatio_original(tokensA, tokensB []string) float64 {
//	lenA, lenB := len(tokensA), len(tokensB)
//	// handle some easy edge cases
//	// Caller has already eliminated the equality case so not doing that again and not doing below
//	//if lenA == 0 && lenB == 0 {
//	//	return 0
//	//}
//	if lenA == 0 || lenB == 0 {
//		return 2.0
//	}
//
//	// Create a 2D slice for storing distances
//	dp := make([][]int, lenA+1)
//	for i := range dp {
//		dp[i] = make([]int, lenB+1)
//	}
//
//	// Initialize the base cases
//	for i := 0; i <= lenA; i++ {
//		dp[i][0] = i
//	}
//	for j := 0; j <= lenB; j++ {
//		dp[0][j] = j
//	}
//
//	// Threshold limits left commented in place in case of future optimizing needs
//	//threshold := lenA
//	// Fill the DP table
//	for i := 1; i <= lenA; i++ {
//		//rowMin := 0
//		for j := 1; j <= lenB; j++ {
//			if tokensA[i-1] == tokensB[j-1] {
//				// Tokens match
//				dp[i][j] = dp[i-1][j-1]
//			} else {
//				// Tokens don't match, compute the cost of insertion, deletion, and substitution
//				dp[i][j] = min(dp[i-1][j], dp[i][j-1], dp[i-1][j-1]) + 1
//			}
//			//rowMin = min(rowMin, dp[i][j])
//		}
//		// Exit early if the minimum distance at this row exceeds the threshold
//		//if rowMin >= threshold {
//		//	return 2
//		//}
//	}
//	//log.Printf("A%3d B%3d dp%3d", lenA, lenB, dp[lenA][lenB])
//	// Return the computed distance as a ratio compared to lengthA
//	return float64(dp[lenA][lenB]) / float64(lenA)
//}
// optimized
var levenshteinWS = struct {
	prev []int
	curr []int
}{}

func levenshteinTokensRatio(tokensA, tokensB []string) float64 {
	lenA, lenB := len(tokensA), len(tokensB)
	if lenA == 0 || lenB == 0 {
		return 2.0
	}

	// Always iterate with the shorter sequence as tokensA to reduce space
	if lenA > lenB {
		tokensA, tokensB = tokensB, tokensA
		lenA, lenB = lenB, lenA
	}
	// scratch int slice reuse
	if cap(levenshteinWS.prev) < lenA+1 {
		levenshteinWS.prev = make([]int, lenA+1)
		levenshteinWS.curr = make([]int, lenA+1)
	}
	prev := levenshteinWS.prev[:lenA+1] //prev := make([]int, lenA+1)
	curr := levenshteinWS.curr[:lenA+1] //curr := make([]int, lenA+1)

	// Initialize base cases
	for i := range prev {
		prev[i] = i
	}

	for j := 1; j <= lenB; j++ {
		curr[0] = j
		for i := 1; i <= lenA; i++ {
			if tokensA[i-1] == tokensB[j-1] {
				curr[i] = prev[i-1]
			} else {
				curr[i] = min(prev[i-1], prev[i], curr[i-1]) + 1
			}
		}
		// Swap slices
		prev, curr = curr, prev
	}

	// prev now holds the final result
	return float64(prev[lenA]) / float64(lenA)
}

// Map month string to number
var monthMap = map[string]time.Month{
	"Jan": 1, "Feb": 2, "Mar": 3, "Apr": 4,
	"May": 5, "Jun": 6, "Jul": 7, "Aug": 8,
	"Sep": 9, "Oct": 10, "Nov": 11, "Dec": 12,
}

func parseNginxTimeFast(line string) (time.Time, error) {
	start := strings.IndexByte(line, '[')
	if start == -1 || start+21 >= len(line) {
		return time.Time{}, errors.New("timestamp start not found or line too short")
	}

	// Fast-path extract substring slice
	t := line[start+1 : start+21] // "02/Jan/2006:15:04:05"
	if len(t) != 20 {
		return time.Time{}, errors.New("unexpected timestamp format length")
	}

	day := t[0:2]
	mon := t[3:6]
	year := t[7:11]
	hour := t[12:14]
	min := t[15:17]
	sec := t[18:20]

	month, ok := monthMap[mon]
	if !ok {
		return time.Time{}, errors.New("invalid month in timestamp")
	}

	// Parse ints manually for performance
	dayInt, _ := strconv.Atoi(day)
	yearInt, _ := strconv.Atoi(year)
	hourInt, _ := strconv.Atoi(hour)
	minInt, _ := strconv.Atoi(min)
	secInt, _ := strconv.Atoi(sec)

	return time.Date(yearInt, month, dayInt, hourInt, minInt, secInt, 0, time.UTC), nil
}

// Ultra HOT and HEAVY code path (heavy == alloc sensitive... interning the strings was a big win)
// works very hard to not create objects and especially to not allow heap escape
func isLikelyIPv4AndPrefix(s string) (bool, string) {
	lenS := len(s)
	dot1 := strings.IndexByte(s, '.')
	if dot1 < 1 || dot1 >= lenS-1 {
		return false, ""
	}

	dot2Rel := strings.IndexByte(s[dot1+1:], '.')
	if dot2Rel < 1 || dot1+1+dot2Rel >= lenS-1 {
		return false, ""
	}
	dot2 := dot1 + 1 + dot2Rel

	// Quick check: is there at least one digit in the third octet?
	// Not validating the full octet, just preventing completely malformed junk.
	for i := dot2 + 1; i < lenS; i++ {
		c := s[i]
		if c == '.' {
			break
		}
		if c >= '0' && c <= '9' {
			prefix := s[:dot2+1] // includes trailing '.'
			return true, prefixInterner.Intern(prefix)
		}
	}

	return false, ""
}

// TODO: replace with size limited LRU?
var prefixInterner = NewPrefixInterner(1024)
var stringInterner = NewLRUInterner(20480)
var filteredToken = stringInterner.Intern("--filtered--")

type PrefixStringInterner struct {
	pool map[string]string
}

// only used for prefix interning now
func NewPrefixInterner(size int) *PrefixStringInterner {
	return &PrefixStringInterner{pool: make(map[string]string, size)}
}
func (si *PrefixStringInterner) Intern(s string) string {
	if interned, ok := si.pool[s]; ok {
		return interned
	}
	cp := strings.Clone(s)
	si.pool[cp] = cp
	return cp
}

type LRUInterner struct {
	capacity int
	pool     map[string]*list.Element
	list     *list.List // to track recency
}

type lruEntry struct {
	key string
}

func NewLRUInterner(cap int) *LRUInterner {
	return &LRUInterner{
		capacity: cap,
		pool:     make(map[string]*list.Element, cap),
		list:     list.New(),
	}
}

func (li *LRUInterner) InternAll(strs []string) []string {
	for i, s := range strs {
		strs[i] = li.Intern(s)
	}
	return strs
}

func (li *LRUInterner) Intern(s string) string {
	if e, ok := li.pool[s]; ok {
		li.list.MoveToFront(e)
		return e.Value.(*lruEntry).key
	}

	// Clone to ensure no backing array retention
	cp := strings.Clone(s)
	en := &lruEntry{key: cp}
	elem := li.list.PushFront(en)
	li.pool[cp] = elem

	if li.list.Len() > li.capacity {
		evicted := li.list.Back()
		if evicted != nil {
			li.list.Remove(evicted)
			delete(li.pool, evicted.Value.(*lruEntry).key)
		}
	}

	return cp
}
func (li *LRUInterner) compactLRU() {
	evictCount := li.list.Len() / 6
	if evictCount < 2000 {
		return
	}
	for i := 0; i < evictCount; i++ {
		elem := li.list.Back()
		if elem == nil {
			break
		}
		entry := elem.Value.(*lruEntry)
		delete(li.pool, entry.key)
		li.list.Remove(elem)
	}
}

// fastFieldsASCIIBuf splits s on ASCII spaces into a reusable scratch slice.
// scratch must be a *[]string that you keep around.
func fastFieldsASCIIBuf(s string, scratch *[]string) []string {
	// ensure capacity at least 50
	if cap(*scratch) < 50 {
		*scratch = make([]string, 0, 50)
	}
	// reset length
	fields := (*scratch)[:0]

	start := -1
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' {
			if start < 0 {
				start = i
			}
		} else if start >= 0 {
			fields = append(fields, s[start:i])
			start = -1
		}
	}
	if start >= 0 {
		fields = append(fields, s[start:])
	}

	// store the used slice back into scratch (so it reuses the backing array next time)
	*scratch = fields
	return fields
}

var factoidHistory []string

const maxFactoidHistory = 50

func newTimestampedFilename(prefix string, suffix string) string {
	return timestampedFilename(prefix, timestampedFileID(time.Now()), suffix)
}

func timestampedFilename(prefix string, id string, suffix string) string {
	return prefix + id + suffix
}

func timestampedFileID(t time.Time) string {
	return fmt.Sprintf("%s_%d", t.UTC().Format("20060102_150405"), os.Getpid())
}

func getWrappedFactoid() string {
	newFact, ranking := facts.NextLogged()
	wrappedFact := "     "
	if strings.TrimSpace(newFact) != "" {
		wrappedFact = " [" + PattyOrange + "]•[default] " + newFact
	}

	// Factoid probability also acts as the panel inclusion rank. Low-frequency
	// tips can appear in the ticker without filling the persistent factoid list.
	if ranking >= 5 {
		// Prepend new fact to the top of the list
		//newFacts := splitFactoidForWidth(newFact, botsDisplayWidth)
		newFacts := []string{newFact}
		factoidHistory = append(newFacts, factoidHistory...)
		if len(factoidHistory) > maxFactoidHistory {
			factoidHistory = factoidHistory[:maxFactoidHistory]
		}
		//if saveFactoidLog && factoidLogFile != nil {
		//	timestamp := time.Now().Format("2006-01-02 15:04:05")
		//	fmt.Fprintf(factoidLogFile, "[%s] %s\n", timestamp, newFact)
		//}
	}

	return wrappedFact
}

func splitFactoidForWidth(text string, maxWidth int) []string {
	var result []string
	var currentLine strings.Builder
	var currentWidth int

	var word strings.Builder
	var wordWidth int

	inBracket := false

	flushWord := func() {
		if word.Len() > 0 {
			if currentWidth+wordWidth > maxWidth {
				// Commit current line and start new one
				if currentLine.Len() > 0 {
					result = append(result, currentLine.String())
					currentLine.Reset()
				}
				currentWidth = 0
			}
			currentLine.WriteString(word.String())
			currentWidth += wordWidth
			word.Reset()
			wordWidth = 0
		}
	}

	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])

		if r == '[' {
			inBracket = true
		}
		if r == ']' {
			inBracket = false
		}

		chunk := text[i : i+size]
		i += size

		word.WriteString(chunk)
		if !inBracket && !unicode.IsSpace(r) {
			wordWidth++
			continue
		}

		// End of a word or whitespace
		flushWord()
		if unicode.IsSpace(r) {
			currentLine.WriteString(chunk)
			currentWidth++
		}
	}

	flushWord()
	if currentLine.Len() > 0 {
		result = append(result, currentLine.String())
	}

	return result
}

func pushFactNow(fName string, args []string) {
	if f, exists := factoidByName[strings.ToLower(fName)]; exists {
		f.args = args
		facts.forced = append(facts.forced, f)
	}
}

// pushFactSnapshotNow evaluates an explicitly requested fact once, queues that
// exact value for the ticker, and returns it for command-result reporting.
// Scheduled fact generation remains deferred through pushFactNow.
func pushFactSnapshotNow(fName string, args []string) (string, bool) {
	f, exists := factoidByName[strings.ToLower(fName)]
	if !exists {
		return "", false
	}

	text := f.Generate(args)
	snapshot := *f
	snapshot.Generate = func(_ []string) string { return text }
	snapshot.args = nil
	snapshot.cache = ""
	facts.forced = append(facts.forced, &snapshot)
	return text, true
}

func pushPrintNow(msg string) {
	facts.forced = append(facts.forced, Once(func(_ []string) string {
		return msg
	}))
}
func pushPrintNowf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	pushPrintNow(msg)
}
func pushErrorNow(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	pushPrintNow(PattyErrorColor + msg + "[default]")
}
func pushSystemNow(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	pushPrintNow(internalFmt(msg))
}
func pushPrintStat(label string, value any) {
	pushPrintNow(fmt.Sprintf("[white]%s:[red]%v[default]", label, value))
}
func pushPrintStatCycles(label string, value any, idx int) {
	eventTime := PattyGraph.logtime.Add(-time.Duration(idx) * time.Minute)
	pushPrintNow(fmt.Sprintf("[white]%s:%v[green]@%s", label, value, eventTime.Format("15:04")))
}

func sliceFromVisibleOffset(s string, visibleOffset int, width int, primeString string) (string, int) {
	var b strings.Builder
	visibleSeen := 0
	visibleCount := 0
	skipping := true
	inBracket := false
	started := false

	b.WriteString(primeString)

	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])

		// Detect tag open/close before anything else
		if r == '[' && !inBracket {
			inBracket = true
		} else if r == ']' && inBracket {
			inBracket = false
			// Append the closing bracket too — but don’t count it as visible
			if !skipping {
				b.WriteString(s[i : i+size])
			}
			i += size
			continue
		}

		if !inBracket {
			// Visible rune
			if skipping {
				visibleSeen++
				if visibleSeen > visibleOffset {
					skipping = false
				}
			} else if visibleCount < width {
				if !started && r == ']' {
					i += size
					continue
				}
				started = true
				b.WriteString(s[i : i+size])
				visibleCount++
			}
		} else {
			// Inside a tag — emit it, but don't count as visible
			if !skipping {
				b.WriteString(s[i : i+size])
			}
		}

		i += size
		if visibleCount >= width {
			break
		}
	}

	return b.String(), visibleCount
}

func visibleRuneWidth(s string) int {
	count := 0
	inBracket := false
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == '[' {
			inBracket = true
		} else if r == ']' && inBracket {
			inBracket = false
			i += size
			continue
		}

		if !inBracket {
			count++
		}

		i += size
	}
	return count
}
func (m *Matcher) burstiness() float64 {
	return burstiness(m.history)
}
func (m *Matcher) spikiness() float64 {
	return spikiness(m.history)
}

func spikiness(history []int) float64 {
	n := len(history)
	if n == 0 {
		return 0
	}
	peak := maxIntSlice(history)
	if peak < 10 {
		return 0 // ignore trivial spikes
	}
	sum := 0
	for _, v := range history {
		sum += v
	}
	mean := float64(sum) / float64(n)
	score := float64(peak) - mean
	if score <= 0 {
		return 0
	}
	return score * math.Log(float64(peak)) // weight larger spikes more
}

// TODO: get wordstats to also call this
func burstiness(history []int) float64 {
	// Cache this expensive operation. Invalidate on push and increments
	depth := float64(len(history))
	if depth <= 1 {
		return 0.0
	}

	// Calculate mean
	sum := 0
	for _, count := range history {
		sum += count
	}
	mean := float64(sum) / depth
	if mean == 0 {
		return 0 // Prevent division by zero
	}

	// Calculate standard deviation
	variance := 0.0
	for _, count := range history {
		diff := float64(count) - mean
		variance += diff * diff
	}
	variance /= depth
	stdDev := math.Sqrt(variance)
	return stdDev / mean
}

var bracketStrip = regexp.MustCompile(`\[.*?\]`)

func stripBrackets(s string) string {
	return bracketStrip.ReplaceAllString(s, "$1")
}

// The Go way to do this is interesting:
//const (
//	_  = iota             // ignore 0
//	KB = 1 << (10 * iota) // 1 KiB
//	MB                    // 1 MiB
//	GB                    // 1 GiB
//	// … etc.
//)
//const maxLogSize = 30 * MB

// NextLogged preserves the historical call site name without writing a separate
// factoid log. Factoids are already emitted through the sidecar JSONL stream.
func (g *FactoidGenerator) NextLogged() (string, int) {
	text, rank, _ := g.Next()
	return text, rank
}

type ringBuffer struct {
	data  [80]int
	size  int
	index int
	full  bool
}

func (r *ringBuffer) Push(v int) {
	r.data[r.index] = v
	r.index = (r.index + 1) % len(r.data)
	if !r.full && r.index == 0 {
		r.full = true
	}
	if !r.full {
		r.size++
	}
}

func (r *ringBuffer) unsafeAt(i int) int {
	return r.data[(r.index-r.Len()+i+len(r.data))%len(r.data)]
}

func (r *ringBuffer) At(i int) int {
	l := r.Len()
	if i < 0 || i >= l {
		panic(fmt.Sprintf(
			"ringBuffer.At: out of bounds (i=%d, Len()=%d, size=%d, index=%d, full=%v)",
			i, l, r.size, r.index, r.full,
		))
	}
	return r.data[(r.index-l+i+len(r.data))%len(r.data)]
}

func (r *ringBuffer) FlatSlice() []int {
	if r.size == 0 {
		return nil
	}
	start := (r.index - r.size + len(r.data)) % len(r.data)
	if start+r.size > len(r.data) {
		return nil // wraps around: not safe for fast path
	}
	return r.data[start : start+r.size]
}

func (r *ringBuffer) Len() int {
	if r.full {
		return len(r.data)
	}
	return r.size
}
func (r *ringBuffer) Latest() int {
	if r.Len() == 0 {
		return 0
	}
	return r.At(r.Len() - 1)
}
func (r *ringBuffer) Penultimate() int {
	if r.Len() < 2 {
		return 0 // or panic, or return a sentinel
	}
	return r.At(r.Len() - 2)
}
func (r *ringBuffer) ReverseSlice() *[]int {
	out := make([]int, r.Len())
	for i := 0; i < len(out); i++ {
		out[i] = r.At(len(out) - 1 - i)
	}
	return &out
}

func (r *ringBuffer) Slice() *[]int {
	out := make([]int, r.Len())
	for i := 0; i < len(out); i++ {
		out[i] = r.At(i)
	}
	return &out
}
func (r *ringBuffer) Reset() {
	r.index = 0
	r.size = 0
	r.full = false
}
func (r *ringBuffer) Total() int {
	sum := 0
	n := r.Len()
	start := r.index - n
	if start < 0 {
		start += len(r.data)
	}
	for i := 0; i < n; i++ {
		sum += r.data[(start+i)%len(r.data)]
	}
	return sum
}
func (r *ringBuffer) SetAt(i int, val int) {
	if i < 0 || i >= r.Len() {
		panic("ringBuffer.SetAt: index out of bounds")
	}

	pos := (r.index - r.Len() + i + len(r.data)) % len(r.data)
	r.data[pos] = val
}

func (r *ringBuffer) AddAt(i int, val int) {
	if i < 0 || i >= r.Len() {
		panic("ringBuffer.SetAt: index out of bounds")
	}
	r.unsafeAddAt(i, val)
	//pos := (r.index - r.Len() + i + len(r.data)) % len(r.data)
	//r.data[pos] += val
}
func (r *ringBuffer) unsafeAddAt(i int, val int) {
	pos := (r.index - r.Len() + i + len(r.data)) % len(r.data)
	r.data[pos] += val
}

func (r *ringBuffer) nFlux(n int) int {
	sum := 0
	limit := min(n, r.Len())
	for i := 0; i < limit; i++ {
		sum += r.unsafeAt(r.Len() - 1 - i)
	}
	return sum
}

func (r *ringBuffer) nFluxAvg(n int) float64 {
	limit := min(n, r.Len())
	if limit == 0 {
		return 0
	}
	sum := 0
	for i := 0; i < limit; i++ {
		sum += r.unsafeAt(r.Len() - 1 - i)
	}
	return float64(sum) / float64(limit)
}

func classifySizeBandKey(bytesValue int) string {
	switch {
	case bytesValue < 100:
		return "<100B" // ultra-tiny (HEAD, 204, malformed)
	case bytesValue < 300:
		return "100–300B" // redirects, bare responses
	case bytesValue < 700:
		return "300–700B" // small error pages
	case bytesValue < 1024:
		return "700B–1K" // edge of 1K
	case bytesValue < 10*1024:
		return "1–10K" // small pages, icons, small JSON
	case bytesValue < 100*1024:
		return "10–100K" // medium API responses
	case bytesValue < 500*1024:
		return "100–500K" // small downloads
	case bytesValue < 1024*1024:
		return "500K–1M" // uploads, attachments
	default:
		return ">1M" // streams, video, dumps
	}
}

/*
*

	The leading spaces are part of the key and part of the display dynamic
	Ordered by the expected measured empirical distribution!!!

	If User-Agent residue were just noisy token count, the distribution should cluster
	smoothly around neighboring buckets. Instead, normal traffic repeatedly lands on
	a few non-adjacent buckets, with B16 dominating and B22/B18 forming the usual
	secondary structure.
*/
func classifyTokenBucket(n int) string {
	switch n {
	case 16:
		return " b16"
	case 22:
		return " b22"
	case 18:
		return " b18"
	case 17:
		return " b17"
	case 24:
		return " b24"
	case 21:
		return " b21"
	case 25:
		return " b25"
	default:
		return " other"
	}
}
