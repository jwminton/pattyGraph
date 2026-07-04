// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"math"
	"sync"
)

// WordStats is the retained state for one interesting key in an InterestingWordMatcher.
//
// Lifecycle:
//   - newWordStats captures the current line source, current status, User-Agent
//     tokens, and starts the key at count/primeFlux 1.
//   - match-time updates increment count, bytes, primeFlux, last-seen log time,
//     retained source examples, capture color, and User-Agent delta state.
//   - push moves the interval count into history, refreshes primeFlux from recent
//     history, clears interval counters, and invalidates cached history views.
//   - stale non-peak entries are recycled through wordStatsPool so the hot path
//     can reuse buffers and source holders without repeated allocation.
//
// WordStats became central because PattyGraph does not only count keys; it keeps
// enough compact state to rank recency, show retained examples, mark capture
// provenance, compare User-Agent shifts, and decide whether a key survives time
// pressure.
//
// The cached fields are maintained at mutation time because count, history[0],
// and count+recent-history are read constantly by ranking and display code.
// Keep changes to these invariants narrow and test-backed.
//
// Invariants:
//   - primeFlux = count + historyBuf.nFlux(fluxDepth)
//   - after push, count and bytes are reset for the new interval
//   - history slice caches are invalidated when history changes
//   - source points to a reusable lineSource owned by this WordStats instance.
type WordStats struct {
	// --- Counters and derived metrics ---
	count            int
	bytes            uint64
	primeFlux        int
	burstiCache      float64
	agentDeltaMetric float64

	// --- History tracking ---
	historyBuf          *ringBuffer
	historyBufferCache  *[]int
	reversedBufferCache *[]int

	// --- Identity / visual metadata ---
	forcedColor           string
	agentTokensFromSource []string

	//tokenBandHistogram [6]int

	// --- State and lifecycle tracking ---
	lastSeenTic int
	lastStatus  string
	source      *lineSource // captureColor,

	// --- Display only ---
	firstIntervalLogLine string
	lastLogLine          string
}

func (w *WordStats) allHistoryIndicator() string {
	switch w.historyLength() {
	case PattyGraph.intervalsCompleted:
		return "+"
	case DefaultHistoryDepth:
		return "+"
	default:
		return " "
	}
}

func (w *WordStats) captureColor() string {
	keyColor := w.forcedColor
	if w.source != nil && w.source.captureColor != "" {
		keyColor = w.source.captureColor
	}
	if keyColor != "" {
		// Stored capture colors include tview brackets; display helpers reapply
		// formatting around the unwrapped color token.
		return keyColor[1 : len(keyColor)-1]
	}
	return ""
}

func (w *WordStats) pattyMonoColor() string {
	return pattyMonoColorForInt(w.historyLength())
}

func (w *WordStats) burstiness() float64 {
	// Cache this expensive operation. Invalidate on push and increments
	if w.burstiCache > 0 {
		return w.burstiCache
	}
	depth := float64(w.historyLength())
	if depth <= 1 {
		return 0.0
	}

	// Its tempting to adjust this according to 'intervals completed so far' but that
	// ends up being too strong of an influence in the early stages. This evens it out
	// appropriately (for how it feels and providing useful "texture" to the metric).
	//depthScale := (1.10 * depth) / DefaultHistoryDepth
	depthScale := depth / DefaultHistoryDepth
	slice := w.historySlice()
	// Calculate mean
	sum := 0
	for _, count := range slice {
		sum += count
	}
	mean := float64(sum) / depth
	if mean == 0 {
		return 0 // Prevent division by zero
	}

	// Calculate standard deviation
	variance := 0.0
	for _, count := range slice {
		diff := float64(count) - mean
		variance += diff * diff
	}
	variance /= depth
	stdDev := math.Sqrt(variance)

	// This has evolved so many times, settling on this as a "final form"
	w.burstiCache = (stdDev / mean) * (1 + w.agentDeltaMetric) * depthScale
	return w.burstiCache
}

// this usage gets reversed!
func (ws *WordStats) historySlice() []int {
	if ws.historyBufferCache == nil {
		ws.historyBufferCache = ws.historyBuf.Slice()
	}
	return *ws.historyBufferCache
}
func (ws *WordStats) reversedHistorySlice() []int {
	if ws.reversedBufferCache == nil {
		ws.reversedBufferCache = ws.historyBuf.ReverseSlice()
	}
	return *ws.reversedBufferCache
}

func (ws *WordStats) historyLength() int {
	return ws.historyBuf.Len()
}
func (ws *WordStats) historyAt(i int) int {
	return ws.historyBuf.At(i)
}

var fluxDepth = 3

func (ws *WordStats) push() {
	ws.historyBuf.Push(ws.count)
	ws.historyBufferCache = nil
	ws.reversedBufferCache = nil

	ws.primeFlux = ws.historyBuf.nFlux(fluxDepth)
	//ws.nFlux = ws.historyBuf.nFlux(fluxDepth)

	// Reset the intervalCount to 0
	ws.count = 0
	ws.bytes = 0
	ws.burstiCache = 0
}

func (ws *WordStats) historyTotal() int { // EXPENSIVE!!!
	if ws.historyLength() != 0 {
		return ws.historyBuf.Total()
	}
	return 0 // Return a default value if history is empty
}

func (ws *WordStats) normalized() float64 { // EXPENSIVE!!!
	historyLen := ws.historyLength()
	if historyLen != 0 {
		return float64(ws.historyTotal()) / float64(historyLen) * pattyScaleFactor * (1 + ws.agentDeltaMetric) // EXPENSIVE!!!
	}
	return float64(ws.count)
}

var poolNews uint64
var poolGets uint64
var poolReturns uint64
var poolGetsMap = make(map[int]uint64, 20)
var poolGetsPerMatcherMap = make(map[uint64]uint64, 20)
var wordStatsPool = sync.Pool{
	New: func() any {
		poolNews++
		return &WordStats{
			historyBuf: &ringBuffer{},
			source:     &lineSource{},
		}
	},
}

func blankWordStats() *WordStats {
	poolGets++
	ws := wordStatsPool.Get().(*WordStats)
	return ws
}
func newWordStats() *WordStats {
	poolGets++
	ws := wordStatsPool.Get().(*WordStats)
	src := ws.source
	rb := ws.historyBuf
	rb.Reset()
	*ws = WordStats{
		count:                 1,
		primeFlux:             1,
		historyBuf:            rb,
		source:                src,
		lastSeenTic:           logicalCycles,
		agentTokensFromSource: currentLine.userAgentTokens,
		lastStatus:            currentLine.respCode,
	}
	*src = lineSource{
		ip:             currentLine.ip,
		captureColor:   currentLine.captureColor,
		captureMatcher: currentLine.captureMatcher,
		ipPrefix:       currentLine.ipPrefix,
		logLine:        currentLine.logLine,
		request:        currentLine.request,
		respCode:       currentLine.respCode,
		bytesValue:     currentLine.bytesValue,
		referer:        currentLine.referer,
	}
	return ws
}
func repopulateWordStats(ws *WordStats) {
	src := ws.source
	rb := ws.historyBuf
	rb.Reset()
	*ws = WordStats{
		count:                 1,
		primeFlux:             1,
		historyBuf:            rb,
		source:                src,
		lastSeenTic:           logicalCycles,
		agentTokensFromSource: currentLine.userAgentTokens,
		lastStatus:            currentLine.respCode,
	}
	*src = lineSource{
		ip:             currentLine.ip,
		captureColor:   currentLine.captureColor,
		captureMatcher: currentLine.captureMatcher,
		ipPrefix:       currentLine.ipPrefix,
		logLine:        currentLine.logLine,
		request:        currentLine.request,
		respCode:       currentLine.respCode,
		bytesValue:     currentLine.bytesValue,
		referer:        currentLine.referer,
	}
}

func (ws *WordStats) Reset() {
	if ws.historyBuf != nil {
		ws.historyBuf.Reset()
	} else {
		ws.historyBuf = &ringBuffer{}
	}
	ws.agentTokensFromSource = nil
	ws.source.captureColor = ""
	ws.source.captureMatcher = ""
}

func recycleWordStats(ws *WordStats) {
	if ws == nil {
		return
	}
	ws.Reset()
	wordStatsPool.Put(ws)
	poolReturns++
}
