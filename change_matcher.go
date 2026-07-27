// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"container/heap"
	"fmt"
	"math"
	"sort"
	"strings"
)

// ChangeMatcher is a push-time system lane. It performs no per-line analysis;
// prePush compares the completed interval with the preceding bounded traffic
// shape while all source matcher and WordStats counters are still intact.
type ChangeMatcher struct {
	*Matcher

	previousShape changeShape
	pendingShape  changeShape
	hasPrevious   bool
	prepared      bool
	scoreReady    bool
	preparedScore float64
	components    []changeComponent
	resetState    changeResetState
}

// changeResetState distinguishes ordinary adjacent comparisons from the two
// comparisons whose Peak distributions are discontinuous after a manual purge.
type changeResetState uint8

const (
	changeResetReady changeResetState = iota
	changeResetPurged
	changeResetRebaseline
)

type changeShape struct {
	lines  float64
	bytes  float64
	errors float64

	averageBytes changeOptionalValue
	marked       changeOptionalValue
	b16          changeOptionalValue

	wordPeaks []changeDistributionEntry
	refPeaks  []changeDistributionEntry
	ipPeaks   []changeDistributionEntry
	wordWave  []changeDistributionEntry
}

type changeOptionalValue struct {
	value     float64
	available bool
}

type changeDistributionEntry struct {
	key   string
	count int
	score int
	peak  bool
}

type changeComponent struct {
	key   string
	score float64
	order int
}

type changeResult struct {
	score      float64
	components []changeComponent
}

const changeTopLimit = defaultSidecarTopLimit
const changeComponentDetailFloor = 30.0

const (
	changeLines = iota
	changeBytes
	changeAverageBytes
	changeErrors
	changeMarked
	changeB16
	changePeakBalance
	changeWordWave
)

var changeComponentNames = [...]string{
	"lines",
	"bytes",
	"avg bytes/line",
	"errs",
	"marked",
	"b16",
	"peak balance",
	"word wave",
}

func NewChangeMatcher() *ChangeMatcher {
	base := NewPredicateMatcher(ChangeMatcherName)
	base.setColor("[" + PattyOrange + "]")
	base.useRegexMatchKeys = true
	base.displayMatchMode = 1
	base.tagIpAction = func() bool { return false }
	base.inlineCommandAction = func() string { return "" }
	base.suppressSelectedSourceLine = true
	return &ChangeMatcher{Matcher: base}
}

// match intentionally adds only one no-op interface call to the observational
// matcher sequence. All useful work happens once per completed interval.
func (m *ChangeMatcher) match() bool {
	return false
}

func (m *ChangeMatcher) prePush() {
	if m == nil || m.prepared || PattyGraph == nil {
		return
	}

	m.pendingShape = captureChangeShape(PattyGraph)
	m.prepared = true
	m.scoreReady = m.hasPrevious
	m.preparedScore = 0
	m.components = m.components[:0]
	m.intervalCount = 0
	m.matchCountsMap = make(map[string]int, len(changeComponentNames))
	m.detailListingCache = ""

	if !m.hasPrevious {
		return
	}

	result := compareChangeShapesWithDistribution(
		m.previousShape,
		m.pendingShape,
		m.resetState == changeResetReady,
	)
	m.preparedScore = result.score
	m.components = append(m.components, result.components...)
	m.intervalCount = roundedChangeScore(result.score)
	for _, component := range m.components {
		m.matchCountsMap[component.key] = roundedChangeScore(component.score)
	}
}

func (m *ChangeMatcher) push() {
	if m == nil {
		return
	}
	m.prePush()

	m.Matcher.pushInterval(m.scoreReady)
	// Ordinary matchers clear their per-interval key counts during push. Change
	// keeps the just-completed component scores visible until the next prePush
	// atomically replaces them.
	m.matchCountsMap = make(map[string]int, len(m.components))
	if m.scoreReady {
		for _, component := range m.components {
			m.matchCountsMap[component.key] = roundedChangeScore(component.score)
		}
	}
	m.detailListingCache = ""

	m.previousShape = m.pendingShape
	m.hasPrevious = true
	switch m.resetState {
	case changeResetPurged:
		m.resetState = changeResetRebaseline
	case changeResetRebaseline:
		m.resetState = changeResetReady
	}
	m.pendingShape = changeShape{}
	m.prepared = false
	m.scoreReady = false
	m.preparedScore = 0
}

// resetPeakBaseline removes Peak-derived components from the next two
// comparisons: populated-to-empty after purge, then empty-to-rebuilt. Scalar
// traffic signals remain valid and continue to contribute normally.
func (m *ChangeMatcher) resetPeakBaseline() {
	if m == nil {
		return
	}
	m.resetState = changeResetPurged
}

// renderSparklineRow shifts the ordinary matcher row to completed-interval
// semantics. The current-value position stays empty because Change has no
// honest in-progress value; the latest completed score and its history carry
// the useful state without inventing a direction against an unfinished minute.
// Formerly called displayString.
func (m *ChangeMatcher) renderSparklineRow() string {
	latest := 0
	if len(m.history) > 0 {
		latest = m.history[len(m.history)-1]
	}

	if m.historySparklineCache == "" {
		m.historySparklineCache = sparklineFromArray(0, 100, m.history)
	}

	format := sharedSystemDisplayFunc(m.displayColor(), m.color)
	return fmt.Sprintf(
		format,
		m.name,
		"-",
		" ",
		formatCounts(latest),
		"|",
		m.historySparklineCache,
	)
}

// renderDetailListing builds Change's lower-left component attribution rows.
// Formerly called displayMatched.
func (m *ChangeMatcher) renderDetailListing() string {
	if m.detailListingCache != "" {
		return m.detailListingCache
	}

	entries := m.sortedComponentEntries(m.displayMatchMode > 1)
	var result strings.Builder
	title := fmt.Sprintf("%s(%s)", m.name, trimmedCounts(len(m.matchCountsMap)))
	latest := 0
	if len(m.history) > 0 {
		latest = m.history[len(m.history)-1]
	}
	result.WriteString(fmt.Sprintf(
		m.displayColor()+"%-16.16s%1s    %4s[-:-]\n",
		title,
		m.expandGlyph(),
		formatCounts(latest),
	))

	m.detailRowCount = 0
	if m.displayMatchMode > 0 {
		for _, entry := range entries {
			if m.displayMatchMode == 1 && dampedChangeComponentScore(entry.count, latest) < changeComponentDetailFloor {
				continue
			}
			result.WriteString(m.componentDisplayLine(entry, latest))
			m.detailRowCount++
		}
	}
	m.detailListingCache = result.String()
	return m.detailListingCache
}

// componentDisplayLine presents attribution as a bounded visual contribution
// rather than an unlabeled number. Every component score is already on the
// same 0-100 scale. The display-only damping ties bar intensity to the final
// Change score while preserving component order and all retained raw values.
func (m *ChangeMatcher) componentDisplayLine(entry matchEntry, overall int) string {
	return fmt.Sprintf(
		m.displayColor()+" %-16.16s %5s  [-:-]\n",
		entry.match,
		renderColoredBar(dampedChangeComponentScore(entry.count, overall)),
	)
}

func dampedChangeComponentScore(component, overall int) float64 {
	if component <= 0 || overall <= 0 {
		return 0
	}
	componentScore := math.Min(100, float64(component))
	overallScore := math.Min(100, float64(overall))
	return componentScore * math.Sqrt(overallScore/100)
}

func (m *ChangeMatcher) sortedComponentEntries(includeZero bool) []matchEntry {
	if len(m.components) > 0 {
		entries := make([]matchEntry, 0, len(m.components))
		// compareChangeShapes already orders components by precise score and
		// stable component priority. Preserve that order when rounded values
		// happen to tie so primary attribution remains truthful.
		for _, component := range m.components {
			count := roundedChangeScore(component.score)
			if count > 0 || includeZero {
				entries = append(entries, matchEntry{match: component.key, count: count})
			}
		}
		return entries
	}

	entries := make([]matchEntry, 0, len(m.matchCountsMap))
	for key, count := range m.matchCountsMap {
		if count > 0 || includeZero {
			entries = append(entries, matchEntry{match: key, count: count})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].count != entries[j].count {
			return entries[i].count > entries[j].count
		}
		return changeComponentOrder(entries[i].match) < changeComponentOrder(entries[j].match)
	})
	return entries
}

func (m *ChangeMatcher) sidecarComponentEntries() []SidecarCountEntry {
	if m == nil || !m.scoreReady {
		return nil
	}
	entries := m.sortedComponentEntries(true)
	out := make([]SidecarCountEntry, 0, len(entries))
	for i, entry := range entries {
		out = append(out, SidecarCountEntry{Key: entry.match, Count: entry.count, Rank: i + 1})
	}
	return out
}

func changeComponentOrder(name string) int {
	for i, candidate := range changeComponentNames {
		if candidate == name {
			return i
		}
	}
	return len(changeComponentNames)
}

func captureChangeShape(m *Monitor) changeShape {
	lines := m.linesMatcher.intervalCount
	bytes := m.bytesMatcher.intervalCount
	shape := changeShape{
		lines:     float64(lines),
		bytes:     float64(bytes),
		errors:    float64(m.errsMatcher.intervalCount),
		wordPeaks: boundedChangeEntries(m.wordsMatcher, true),
		refPeaks:  boundedChangeEntries(m.refsMatcher, true),
		ipPeaks:   boundedChangeEntries(m.ipsMatcher, true),
	}
	topWords := boundedChangeEntries(m.wordsMatcher, false)
	shape.wordWave = make([]changeDistributionEntry, 0, len(topWords))
	for _, entry := range topWords {
		if !entry.peak {
			shape.wordWave = append(shape.wordWave, entry)
		}
	}
	if lines > 0 {
		shape.averageBytes = changeOptionalValue{value: float64(bytes) / float64(lines), available: true}
		shape.marked = changeOptionalValue{
			value:     float64(m.linesMatcher.matchCountsMap["marked"]) * 100 / float64(lines),
			available: true,
		}
		shape.b16 = changeOptionalValue{
			value:     float64(m.linesMatcher.matchCountsMap[" b16"]) * 100 / float64(lines),
			available: true,
		}
	}
	return shape
}

func compareChangeShapes(reference, selected changeShape) changeResult {
	return compareChangeShapesWithDistribution(reference, selected, true)
}

func compareChangeShapesWithDistribution(reference, selected changeShape, includeDistribution bool) changeResult {
	components := make([]changeComponent, 0, len(changeComponentNames))
	components = append(components,
		newChangeComponent(changeLines, relativeChangeScore(reference.lines, selected.lines, 100, 0.25)),
		newChangeComponent(changeBytes, relativeChangeScore(reference.bytes, selected.bytes, 1024*1024, 0.25)),
	)
	if reference.averageBytes.available && selected.averageBytes.available {
		components = append(components, newChangeComponent(changeAverageBytes, relativeChangeScore(
			reference.averageBytes.value, selected.averageBytes.value, 128, 0.25,
		)))
	}
	components = append(components,
		newChangeComponent(changeErrors, relativeChangeScore(reference.errors, selected.errors, 10, 0.50)),
	)
	if reference.marked.available && selected.marked.available {
		components = append(components, newChangeComponent(changeMarked, pointChangeScore(
			reference.marked.value, selected.marked.value, 4,
		)))
	}
	if reference.b16.available && selected.b16.available {
		components = append(components, newChangeComponent(changeB16, pointChangeScore(
			reference.b16.value, selected.b16.value, 12,
		)))
	}
	if includeDistribution {
		peakBalance := math.Max(
			softChangeScore(changeDistributionDistance(reference.wordPeaks, selected.wordPeaks), 0.10),
			math.Max(
				0.80*softChangeScore(changeDistributionDistance(reference.refPeaks, selected.refPeaks), 0.12),
				0.40*softChangeScore(changeDistributionDistance(reference.ipPeaks, selected.ipPeaks), 0.25),
			),
		)
		components = append(components,
			newChangeComponent(changePeakBalance, peakBalance),
			newChangeComponent(changeWordWave, 0.75*softChangeScore(
				changeDistributionDistance(reference.wordWave, selected.wordWave), 0.30,
			)),
		)
	}

	sort.SliceStable(components, func(i, j int) bool {
		if components[i].score != components[j].score {
			return components[i].score > components[j].score
		}
		return components[i].order < components[j].order
	})

	primary := components[0].score
	secondary := 0.0
	tertiary := 0.0
	if len(components) > 1 {
		secondary = components[1].score / 100
	}
	if len(components) > 2 {
		tertiary = components[2].score / 100
	}
	composite := math.Min(100, primary+(100-primary)*(0.15*secondary+0.05*tertiary))
	return changeResult{score: composite * composite / 100, components: components}
}

func newChangeComponent(order int, score float64) changeComponent {
	return changeComponent{key: changeComponentNames[order], score: score, order: order}
}

func relativeChangeScore(reference, selected, floor, halfScale float64) float64 {
	denominator := math.Max(math.Max(math.Abs(reference), math.Abs(selected)), floor)
	return softChangeScore(math.Abs(selected-reference)/denominator, halfScale)
}

func pointChangeScore(reference, selected, halfScale float64) float64 {
	return softChangeScore(math.Abs(selected-reference), halfScale)
}

func softChangeScore(value, halfScale float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return 0
	}
	squared := value * value
	return 100 * squared / (squared + halfScale*halfScale)
}

func roundedChangeScore(value float64) int {
	return int(math.Round(math.Max(0, math.Min(100, value))))
}

func changeDistributionDistance(reference, selected []changeDistributionEntry) float64 {
	referenceTotal := 0
	selectedTotal := 0
	referenceCounts := make(map[string]int, len(reference))
	selectedCounts := make(map[string]int, len(selected))
	for _, entry := range reference {
		if entry.key != "" && entry.count >= 0 {
			referenceCounts[entry.key] += entry.count
			referenceTotal += entry.count
		}
	}
	for _, entry := range selected {
		if entry.key != "" && entry.count >= 0 {
			selectedCounts[entry.key] += entry.count
			selectedTotal += entry.count
		}
	}
	if referenceTotal == 0 && selectedTotal == 0 {
		return 0
	}
	if referenceTotal == 0 || selectedTotal == 0 {
		return 1
	}

	difference := 0.0
	seen := make(map[string]struct{}, len(referenceCounts)+len(selectedCounts))
	for key, count := range referenceCounts {
		seen[key] = struct{}{}
		difference += math.Abs(float64(count)/float64(referenceTotal) - float64(selectedCounts[key])/float64(selectedTotal))
	}
	for key, count := range selectedCounts {
		if _, exists := seen[key]; exists {
			continue
		}
		difference += float64(count) / float64(selectedTotal)
	}
	return difference / 2
}

func boundedChangeEntries(m *InterestingWordMatcher, peaksOnly bool) []changeDistributionEntry {
	if m == nil {
		return nil
	}
	h := make(changeEntryHeap, 0, changeTopLimit)
	if peaksOnly {
		for _, key := range m.peakWords {
			stats := m.wordFrequency[key]
			if stats == nil {
				continue
			}
			pushBoundedChangeEntry(&h, changeDistributionEntry{
				key: key, count: stats.count, score: stats.count + stats.primeFlux, peak: true,
			})
		}
	} else {
		// The stream's scored LRU is maintained while entries are observed and
		// keeps a small candidate corpus for display ranking. Reusing it here
		// avoids a second full WordStats map scan at every interval boundary;
		// InterestingWordMatcher.push already owns that necessary lifecycle pass.
		for elem := m.lruTracker.list.Front(); elem != nil; elem = elem.Next() {
			key := elem.Value.(*ScoredEntry).key
			stats := m.wordFrequency[key]
			if stats == nil {
				continue
			}
			pushBoundedChangeEntry(&h, changeDistributionEntry{
				key: key, count: stats.count, score: stats.count + stats.primeFlux, peak: m.peakWordsSet[key],
			})
		}
	}

	entries := make([]changeDistributionEntry, len(h))
	copy(entries, h)
	sort.Slice(entries, func(i, j int) bool {
		return changeEntryBetter(entries[i], entries[j])
	})
	return entries
}

func pushBoundedChangeEntry(h *changeEntryHeap, entry changeDistributionEntry) {
	if len(*h) < changeTopLimit {
		heap.Push(h, entry)
		return
	}
	if changeEntryBetter(entry, (*h)[0]) {
		(*h)[0] = entry
		heap.Fix(h, 0)
	}
}

func changeEntryBetter(left, right changeDistributionEntry) bool {
	if left.score != right.score {
		return left.score > right.score
	}
	return left.key < right.key
}

// changeEntryHeap keeps the worst retained candidate at index zero.
type changeEntryHeap []changeDistributionEntry

func (h changeEntryHeap) Len() int { return len(h) }
func (h changeEntryHeap) Less(i, j int) bool {
	if h[i].score != h[j].score {
		return h[i].score < h[j].score
	}
	return h[i].key > h[j].key
}
func (h changeEntryHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *changeEntryHeap) Push(value interface{}) {
	*h = append(*h, value.(changeDistributionEntry))
}
func (h *changeEntryHeap) Pop() interface{} {
	old := *h
	last := old[len(old)-1]
	*h = old[:len(old)-1]
	return last
}
