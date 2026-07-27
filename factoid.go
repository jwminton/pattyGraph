// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"time"
)

// factoid.go is PattyGraph's observation catalog.
//
// Factoids are not only ticker text. Each named factoid is also a reverse map
// from visible runtime language back to the implementation that produced it.
// Stable names matter because --help facts, ticker output, sidecar summaries,
// and debugging conversations can all use those names as handles into the code.
// Factoid entries are visually separated by colored dividers, so the text inside
// each entry should be greedy with space: prefer terse labels, compact symbols,
// and trimmed count formatting over explanatory prose.
//
// Expect this file to stay broader and more experimental than the core ingestion
// and matcher paths: new observations often start here before they prove whether
// they deserve a larger UI surface or a dedicated document.
var factoidByName = map[string]*Factoid{}

type FactoidFunc func(args []string) string
type FactoidSchedule func(cycle int, lastSeen int, shown bool) bool

// Low-rank observations such as tips may pass through the live ticker without
// becoming retained factoid-panel or PattyLog interval content.
const minimumFactoidRetentionRank = 5

func retainFactoidRank(rank int) bool {
	return rank >= minimumFactoidRetentionRank
}

// Factoid describes one named observation PattyGraph can surface. Condition
// controls when the observation is eligible; Generate reads live process state
// and returns already-marked-up display text.
type Factoid struct {
	FID         int
	Name        string
	Generate    FactoidFunc
	Condition   FactoidSchedule
	probability int // schedule percentage and retained-output inclusion rank
	LastSeen    int
	Shown       bool
	cache       string
	args        []string
}

// Scheduled is the single constructor for factoids. The named wrappers below
// preserve the compact registration style while making the scheduling policy
// explicit and reusable. probability is intentionally retained as both the
// schedule percentage and the rank used when deciding whether a shown factoid is
// important enough for the factoid history panel and PattyLog interval records.
func Scheduled(probability int, schedule FactoidSchedule, f FactoidFunc) *Factoid {
	return &Factoid{
		Generate:    f,
		probability: probability,
		Condition:   schedule,
	}
}

func CycleSchedule(mod, offset int) FactoidSchedule {
	return func(cycle, _ int, _ bool) bool {
		return (cycle-offset)%mod == 0
	}
}

func RandomSchedule(percent int) FactoidSchedule {
	return func(cycle, lastSeen int, _ bool) bool {
		if lastSeen > 0 && cycle-lastSeen < 10 {
			return false
		}
		if percent <= 0 {
			return false
		}
		if percent >= 100 {
			return true
		}
		return rand.Intn(100) < percent
	}
}

func RepeatingSchedule(percent int) FactoidSchedule {
	return func(cycle, lastSeen int, _ bool) bool {
		if lastSeen > 0 && cycle-lastSeen <= 1 {
			return false
		}
		if percent <= 0 {
			return false
		}
		if percent >= 100 {
			return true
		}
		return rand.Intn(100) < percent
	}
}

func OnceSchedule() FactoidSchedule {
	return func(_ int, _ int, shown bool) bool {
		return !shown
	}
}

func EverySchedule(n int) FactoidSchedule {
	return func(cycle, lastSeen int, _ bool) bool {
		return (cycle - lastSeen) >= n
	}
}

func AlwaysSchedule() FactoidSchedule {
	return func(_, _ int, _ bool) bool {
		return true
	}
}

func DirectOnlySchedule() FactoidSchedule {
	return func(_, _ int, _ bool) bool {
		return false
	}
}

func CycleMod(mod, offset int, f FactoidFunc) *Factoid {
	return Scheduled(0, CycleSchedule(mod, offset), f)
}

func Random(percent int, f FactoidFunc) *Factoid {
	return Scheduled(percent, RandomSchedule(percent), f)
}

// Repeating has a deliberately shorter cooldown than Random so operator-facing
// forced/background observations can recur without waiting ten ticker cycles.
func Repeating(percent int, f FactoidFunc) *Factoid {
	return Scheduled(percent, RepeatingSchedule(percent), f)
}

func Once(f FactoidFunc) *Factoid {
	return Scheduled(100, OnceSchedule(), f)
}

func Every(n int, f FactoidFunc) *Factoid {
	return Scheduled(100, EverySchedule(n), f)
}

func Always(f FactoidFunc) *Factoid {
	return Scheduled(100, AlwaysSchedule(), f)
}

// DirectOnly factoids are addressable by name but never enter the random or
// periodic background stream. Their high rank keeps requested values in the
// retained factoid panel after they pass through the ticker.
func DirectOnly(f FactoidFunc) *Factoid {
	return Scheduled(100, DirectOnlySchedule(), f)
}

type FactoidGenerator struct {
	forced []*Factoid
	facts  []*Factoid
	cycle  int
}

// Add registers a factoid under a stable dotted name. That name is the bridge
// between what an operator sees and where the observation lives in source.
func (g *FactoidGenerator) Add(f *Factoid, name ...string) {
	f.FID = len(g.facts)
	var key string
	if name != nil {
		if name[0] != "" {
			key = name[0]
		}
		if len(name) > 1 && name[1] != "" {
			key += "." + name[1]
		}
	} else {
		key = "fact." + strconv.Itoa(f.FID)
	}
	f.Name = key
	g.facts = append(g.facts, f)
	factoidByName[strings.ToLower(f.Name)] = f
}

var doRandomFact bool

func (g *FactoidGenerator) Next() (string, int, string) {
	return g.next(true)
}

func (g *FactoidGenerator) NextBackground() (string, int, string) {
	return g.next(false)
}

// next picks the next eligible observation for the ticker/background stream.
// Forced factoids are used for direct operator feedback; random/cyclic factoids
// keep the ticker from becoming a fixed wall of status text.
func (g *FactoidGenerator) next(includeForced bool) (string, int, string) {

	if includeForced && len(g.forced) > 0 {
		for _, f := range g.forced {
			//if f.Condition(g.cycle, f.LastSeen, f.Shown) {
			g.forced = g.forced[1:]
			f.cache = f.Generate(f.args)
			f.args = nil
			return f.cache, f.probability, f.Name
			//}
		}
	}

	g.cycle++

	var ready []*Factoid
	// Give it 10 shots
	if doRandomFact {
		for i := 0; i < 10; i++ {
			for _, f := range g.facts {
				if f.Condition(g.cycle, f.LastSeen, f.Shown) {
					ready = append(ready, f)
				}
			}

			if len(ready) > 0 {
				// Randomly choose one ready factoid
				chosen := ready[rand.Intn(len(ready))]
				value := chosen.Generate(chosen.args)
				chosen.args = nil
				// let it still bail at the last minute and let someone else win next
				if value != "" {
					chosen.cache = value
					chosen.LastSeen = g.cycle
					chosen.Shown = true
					return value, chosen.probability, chosen.Name
				}
			}
		}
	}
	return "   ", 0, ""
}

func maxIntSlice(s []int) int {
	if len(s) == 0 {
		return 0
	}
	max := s[0]
	for _, v := range s {
		if v > max {
			max = v
		}
	}
	return max
}

const memGcColor = "[plum]"
const toolColor = "[palegreen]"
const internalsColor = "[green]"
const ipColor = "[skyblue]"
const selectedTickerBg = "[:#000040]"
const defaultTickerBg = "[:#101010]"

var matcherMarchCount = 0

// NewFactoidGenerator is the main observation catalog. Keep registrations close
// to the state they describe when practical, but prefer stable factoid names over
// clever grouping: the names are useful external handles.
func NewFactoidGenerator() *FactoidGenerator {
	g := &FactoidGenerator{}

	// Live stat closures
	//linesRead := func() int { return PattyGraph.totalLines }
	botsForked := func() int { return botsMigrated }
	peakErrs := func() int { return maxIntSlice(PattyGraph.errsMatcher.history) }
	peakBytes := func() string { return formatBytes(maxIntSlice(PattyGraph.bytesMatcher.history)) }
	peakLines := func() string { return formatCounts(maxIntSlice(PattyGraph.linesMatcher.history)) }

	// Register startup, rotating, and periodic facts
	//cleanText := strings.ReplaceAll(baseText, "[", "⟦")
	//cleanText = strings.ReplaceAll(cleanText, "]", "⟧")
	welcome := Random(4, func(_ []string) string {
		return "           [white]▁▂▃▄▅▆▇█ " +
			gradientText("pattyGraph ",
				[]string{"red", "orangered", PattyOrange, "green", "blue", "indigo", "violet"}) +
			gradientText(PattyGraphVersion,
				[]string{"red", "orangered", PattyOrange, "green", "blue", "indigo", "violet"}) +
			" [white]█▇▆▅▄▃▂▁ "
		//return "Welcome to PattyGraph " + PattyGraphVersion
	})

	logReadIn := Once(func(_ []string) string {
		return fmt.Sprintf(toolFmt("Init(%s):%s/%slines/%dmin"),
			formatShortDuration(logLoadDuration),
			formatBytes(startBytesRead),
			formatCountsUint64(logLoadLinecount),
			logLoadIntervalCount,
		)
	})
	// Welcome messages
	g.forced = []*Factoid{welcome, logReadIn}

	g.Add(DirectOnly(func(args []string) string {
		if len(args) == 0 || args[0] == "" {
			return ""
		}
		return internalFmt("Note:") + "[white] " + args[0] + "[-:-:-:-]"
	}), "print")
	g.Add(DirectOnly(func(_ []string) string {
		return internalFmt("Peak memory reset: starting fresh")
	}), "model", "peakReset")

	g.Add(Random(1, func(_ []string) string {
		return fmt.Sprintf(toolFmt("Init(%s):%s/%slines"),
			formatShortDuration(logLoadDuration),
			formatBytes(startBytesRead),
			formatCountsUint64(logLoadLinecount),
		)
	}), "init", "lines")
	g.Add(Random(1, func(_ []string) string {
		return fmt.Sprintf(toolFmt("Init(%s):%dmin history"),
			formatShortDuration(logLoadDuration),
			logLoadIntervalCount,
		)
	}), "init", "history")

	g.Add(Random(5, func(_ []string) string {
		if len(poolGetsMap) == 0 {
			return ""
		}
		type entry struct {
			gets uint64
			freq uint64
		}
		entries := make([]entry, 0, len(poolGetsPerMatcherMap))
		for gets, freq := range poolGetsPerMatcherMap {
			entries = append(entries, entry{gets, freq})
		}
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].freq == entries[j].freq {
				return entries[i].gets > entries[j].gets
			}
			return entries[i].freq > entries[j].freq
		})
		limit := 5
		if len(entries) < 5 {
			limit = len(entries)
		}
		var b strings.Builder
		b.WriteString("wsGets/matcher:")
		for i := 0; i < limit; i++ {
			if i > 0 {
				b.WriteByte(' ')
			}
			fmt.Fprintf(&b, toolFmt("%s×%s"), formatCountsUint64(entries[i].gets), formatCountsUint64(entries[i].freq))
		}
		return b.String()
	}), "metrics", "poolPerMatcher")

	g.Add(Random(5, func(_ []string) string {
		if len(poolGetsMap) == 0 {
			return ""
		}
		type entry struct {
			gets int
			freq uint64
		}
		entries := make([]entry, 0, len(poolGetsMap))
		for gets, freq := range poolGetsMap {
			entries = append(entries, entry{gets, freq})
		}
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].freq == entries[j].freq {
				return entries[i].gets > entries[j].gets
			}
			return entries[i].freq > entries[j].freq
		})
		limit := 5
		if len(entries) < 5 {
			limit = len(entries)
		}
		var b strings.Builder
		b.WriteString("wsPoolGets:")
		for i := 0; i < limit; i++ {
			if i > 0 {
				b.WriteByte(' ')
			}
			fmt.Fprintf(&b, toolFmt("%s×%s"), trimmedCounts(entries[i].gets), formatCountsUint64(entries[i].freq))
		}
		return b.String()
	}), "metrics", "pool")
	//g.Add(Random(10, func(_ []string) string {
	//	if PattyGraph.totalLines == 0 {
	//		return ""
	//	}
	//	avgWS := float64(poolGets) / float64(PattyGraph.totalLines)
	//	avgUATokens := float64(totalAgentTokenCount) / float64(PattyGraph.totalLines)
	//
	//	return internalFmt(fmt.Sprintf(internalFmt("avg.ws:%.2f  avg.uaTokens:%.2f"), avgWS, avgUATokens))
	//}), "metrics", "wsAverages")

	g.Add(Random(10, func(_ []string) string {
		if len(uaCardinalityMap) == 0 {
			return ""
		}
		type entry struct {
			tokens int
			count  uint64
		}
		entries := make([]entry, 0, len(uaCardinalityMap))
		for k, v := range uaCardinalityMap {
			entries = append(entries, entry{tokens: k, count: v})
		}
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].count == entries[j].count {
				return entries[i].tokens < entries[j].tokens
			}
			return entries[i].count > entries[j].count
		})

		limit := 5
		if len(entries) < limit {
			limit = len(entries)
		}
		var b strings.Builder
		b.WriteString("UA Bands:")
		for i := 0; i < limit; i++ {
			if i > 0 {
				b.WriteByte(' ')
			}
			fmt.Fprintf(&b, toolFmt("%d×%s"), entries[i].tokens, formatCountsUint64(entries[i].count))
		}
		return b.String()
	}), "metrics", "UABands")

	g.Add(Random(10, func(_ []string) string {
		lines := PattyGraph.intervalLines
		if lines == 0 {
			return ""
		}
		marked := PattyGraph.linesMatcher.matchCountsMap["marked"]
		percent := (float64(marked) / float64(lines)) * 100
		return fmt.Sprintf(internalFmt("Marked:%.1f%%"), percent)
	}), "metrics", "marked")

	g.Add(Random(3, func(_ []string) string {
		return fmt.Sprintf(toolFmt("Push:%d"), pattyPushFactor)
	}), "settings", "push")
	g.Add(Random(3, func(_ []string) string {
		return fmt.Sprintf(toolFmt("Grace:%d"), pattyGracePeriod)
	}), "settings", "grace")
	g.Add(Random(3, func(_ []string) string {
		return fmt.Sprintf(toolFmt("Scale:%.1f"), pattyScaleFactor)
	}), "settings", "scale")
	g.Add(Random(3, func(_ []string) string {
		return fmt.Sprintf(internalFmt("fluxDepth:%d"), fluxDepth)
	}), "settings", "flux")
	g.Add(Random(3, func(_ []string) string {
		return fmt.Sprintf(toolFmt("Pressure p:%d g:%d s:%.1f f:%d"),
			pattyPushFactor,
			pattyGracePeriod,
			pattyScaleFactor,
			fluxDepth)
	}), "settings", "pressure")
	g.Add(Random(4, func(_ []string) string {
		c := AutobotColors[(colorIndex+1)%len(AutobotColors)]
		return fmt.Sprintf(wrapFmt("Next Color:%s", c), c[1:len(c)-1])
	}), "settings", "color-index")

	g.Add(Random(10, func(_ []string) string {
		return fmt.Sprintf(ipFmt("Active Prefixes:%d"),
			PattyGraph.ipsMatcher.ipScratch.activePrefixesCountMetric)
	}), "ips", "active")

	// TODO: make this one more interesting than 17.xx every time
	//g.Add(Random(10, func(_ []string) string {
	//	avg := float64(PattyGraph.totalAgentTokens) / float64(PattyGraph.totalLines)
	//	return fmt.Sprintf(toolFmt("Avg tokens/UA:%.2f"), avg)
	//}))
	g.Add(Random(5, func(_ []string) string {
		l := len(PattyGraph.matchers)
		return fmt.Sprintf(toolFmt("Matchers:%d"),
			l)
	}), "matcher", "count")
	g.Add(Random(5, func(_ []string) string {
		return fmt.Sprintf(toolFmt("Shared min/max:%d/%d"),
			PattyGraph.overallMin, PattyGraph.overallMax)
	}), "metrics", "sharedRange")

	/********* MEM & GC STATS ************/
	g.Add(Random(20, func(_ []string) string {
		var stats debug.GCStats
		debug.ReadGCStats(&stats)

		intervals := PattyGraph.intervalsCompleted
		gcs := stats.NumGC
		rate := 0.0
		if intervals > 0 {
			rate = float64(gcs) / float64(intervals)
		}
		return fmt.Sprintf(memFmt("GCs/Interval:%.1f"), rate)
	}), "gc", "interval")
	g.Add(Random(20, func(_ []string) string {
		var stats debug.GCStats
		debug.ReadGCStats(&stats)
		gcs := stats.NumGC
		lines := PattyGraph.totalLines
		per := 0.0
		if gcs > 0 {
			per = float64(lines) / float64(gcs)
		}
		return fmt.Sprintf(memFmt("Lines/GC:%s"), strings.TrimSpace(formatCounts(int(per))))
	}), "gc", "lines")

	// GC Cycles
	//g.Add(Random(2, func(_ []string) string {
	//	var stats debug.GCStats
	//	debug.ReadGCStats(&stats)
	//
	//	gcs := stats.NumGC
	//	return fmt.Sprintf("GC cycles:[green]%s[default]", formatCountsUint64(uint64(gcs)))
	//}))

	// Total Pause Time
	//g.Add(Random(1, func(_ []string) string {
	//	var stats debug.GCStats
	//	debug.ReadGCStats(&stats)
	//	return fmt.Sprintf("Total GC pause: [palegreen]%s[default]", formatShortDuration(stats.PauseTotal))
	//}))

	// Last Pause Duration
	//g.Add(Random(5, func(_ []string) string {
	//	var stats debug.GCStats
	//	debug.ReadGCStats(&stats)
	//	if len(stats.Pause) > 0 {
	//		last := stats.Pause[len(stats.Pause)-1]
	//		return fmt.Sprintf("Last GC pause:[green]%s[default]", formatShortDuration(last))
	//	}
	//	return "" // tells Generator we changed our mind last minute... don't use this factoid
	//}))
	g.Add(Random(5, func(_ []string) string {
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		return fmt.Sprintf(memFmt("Mallocs:%s/Frees:%s"),
			formatCountsUint64(mem.Mallocs), formatCountsUint64(mem.Frees))
	}), "mem", "ratio")
	g.Add(Random(5, func(_ []string) string {
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		return fmt.Sprintf(memFmt("Mallocs-Frees:%s"),
			formatCountsUint64(mem.Mallocs-mem.Frees))
	}), "mem", "diff")

	//g.Add(Random(50, func(_ []string) string {
	//	var mem runtime.MemStats
	//	runtime.ReadMemStats(&mem)
	//	return fmt.Sprintf("HeapObjs:[green]%s[default] / Stack:[green]%s[default]",
	//		formatCountsUint64(mem.HeapObjects),
	//		formatBytes64(mem.StackInuse))
	//}))
	g.Add(Random(5, func(_ []string) string {
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		return fmt.Sprintf(memFmt("Allocs:%s"), formatBytes64(mem.TotalAlloc))
	}), "mem", "allocs")
	g.Add(Random(5, func(_ []string) string {
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		return fmt.Sprintf(memFmt("Heap:%s"), formatBytes64(mem.HeapAlloc))
	}), "mem", "heap")
	//g.Add(Random(5, func(_ []string) string {
	//	var m runtime.MemStats
	//	runtime.ReadMemStats(&m)
	//	return fmt.Sprintf(memFmt("SysMem:%s"), formatBytes64(m.Sys))
	//}), "mem", "sys")
	g.Add(Random(5, func(_ []string) string {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return fmt.Sprintf(memFmt("Heap InUse/Idle:%s/%s"),
			formatBytes64(m.HeapInuse),
			formatBytes64(m.HeapIdle),
		)
	}), "mem", "useIdle")
	/**************** MISC *******************/
	g.Add(Random(5, func(_ []string) string {
		v := "Log size:[palegreen]unavailable[default]"
		if info, err := os.Stat(PattyGraph.filePath); err == nil {
			v = strings.TrimSpace(formatBytes64(uint64(info.Size())))
		}
		return fmt.Sprintf(toolFmt("Log size:%s"), v)
	}), "log", "size")
	g.Add(Random(3, func(_ []string) string {
		if generateSidecarJSONL {
			return fmt.Sprintf(toolFmt("Sidecar:on schema:%d"), SidecarSchemaVersion)
		}
		return toolFmt("Sidecar:off")
	}), "sidecar", "status")
	g.Add(Random(3, func(_ []string) string {
		jsonState := "off"
		if generateSidecarJSONL {
			jsonState = filepath.Base(PattyGraph.SidecarDefaultPath())
		}
		controlState := "off"
		if enableControlFile {
			controlState = filepath.Base(controlFilePath())
		}
		saveState := "cwd"
		if PattyGraph.pattyConfig != nil && PattyGraph.pattyConfig.saveDir != "" {
			saveState = filepath.Base(filepath.Clean(PattyGraph.pattyConfig.saveDir))
		}
		failState := ""
		if sidecarWriteFailures > 0 {
			failState = fmt.Sprintf(" fail:%d/%d", sidecarWriteFailures, sidecarWriteFailureLimit)
		}
		return fmt.Sprintf(internalFmt("Output json:%s control:%s save:%s%s"),
			jsonState,
			controlState,
			saveState,
			failState)
	}), "output", "paths")
	//g.Add(Random(5, func(_ []string) string {
	//	return fmt.Sprintf("Strings:[palegreen]%s[default]",
	//		strings.TrimSpace(formatCounts(stringInterner.list.Len())))
	//}))
	//g.Add(Random(5, func(_ []string) string {
	//	prefixes := len(PattyGraph.ipsMatcher.ipScratch.prefixToIPs)
	//	ips := len(PattyGraph.ipsMatcher.wordFrequency)
	//	return fmt.Sprintf("Prefixes/IPs:[palegreen]%s[default]/[palegreen]%s[default]",
	//		formatCounts(prefixes), formatCounts(ips))
	//}))
	g.Add(Random(10, func(_ []string) string {
		totalPrefixes := len(PattyGraph.ipsMatcher.ipScratch.prefixToIPs)
		totalIPs := 0
		for _, ipset := range PattyGraph.ipsMatcher.ipScratch.prefixToIPs {
			totalIPs += len(ipset)
		}
		avg := 0.0
		if totalPrefixes > 0 {
			avg = float64(totalIPs) / float64(totalPrefixes)
		}
		return fmt.Sprintf(ipFmt("Avg IPs/Prefix:%.2f"), avg)
	}), "ips", "grouping")

	g.Add(Random(10, func(_ []string) string {
		return fmt.Sprintf(ipFmt("Prefixes:%s"),
			strings.TrimSpace(formatCounts(len(PattyGraph.ipsMatcher.ipScratch.prefixToIPs))))
	}), "ips", "prefixes")
	generator := g
	g.Add(Random(1, func(_ []string) string {
		return fmt.Sprintf(internalFmt("Factoids:%d"),
			len(generator.facts))
	}), "factoids")
	g.Add(Random(1, func(_ []string) string {
		return fmt.Sprintf(internalFmt("%s:%s"), PattyGraphName, PattyGraphVersion)
	}), "version", "pattyGraph")
	g.Add(Random(1, func(_ []string) string {
		return fmt.Sprintf(internalFmt("%s"), runtime.Version())
	}), "version", "go")
	//g.Add(Random(5, func(_ []string) string {
	//	wordCount := len(PattyGraph.wordsMatcher.wordFrequency)
	//	ipCount := len(PattyGraph.ipsMatcher.wordFrequency)
	//	ratio := 0.0
	//	if ipCount > 0 {
	//		ratio = float64(wordCount) / float64(ipCount)
	//	}
	//	return fmt.Sprintf(ipFmt("Word/IP ratio:%.2f"), ratio)
	//}))
	//g.Add(Random(5, func(_ []string) string {
	//	refCount := len(PattyGraph.refsMatcher.wordFrequency)
	//	ipCount := len(PattyGraph.ipsMatcher.wordFrequency)
	//	ratio := 0.0
	//	if ipCount > 0 {
	//		ratio = float64(refCount) / float64(ipCount)
	//	}
	//	return fmt.Sprintf(ipFmt("Ref/IP ratio:%.2f"), ratio)
	//}))
	g.Add(Random(5, func(_ []string) string {
		calculatedUptime := time.Now().Sub(startTime)
		return fmt.Sprintf(internalFmt("Uptime:%s"), formatUptime(calculatedUptime))
	}), "uptime")
	g.Add(Random(5, func(_ []string) string {
		bf := botsForked()
		if bf == 0 {
			return "" // don't include this factoid if nothing's been injected
		}
		return fmt.Sprintf(PattyBotsColor+"Matchers auto-injected:[palegreen]%d[default]", bf)
	}), "matcher", "injected")
	g.Add(Random(5, func(_ []string) string {
		return fmt.Sprintf(PattyErrorColor+"Peak errs:%d[default]/min", peakErrs())
	}), "traffic", "peakErrs")
	//g.Add(Random(1, func(_ []string) string {
	//	return fmt.Sprintf(PattyLinesColor+"Total lines:%s[default]", strings.TrimSpace(formatCountsUint64(PattyGraph.totalLines)))
	//}))
	g.Add(Random(5, func(_ []string) string {
		return fmt.Sprintf(PattyLinesColor+"Peak lines:%s[default]/min", peakLines())
	}), "traffic", "peakLines")
	g.Add(Random(1, func(_ []string) string {
		return fmt.Sprintf(PattyBytesColor+"Total bytes:%s[default]", strings.TrimSpace(formatBytes64(PattyGraph.totalBytes)))
	}), "traffic", "totalBytes")
	g.Add(Random(5, func(_ []string) string {
		return fmt.Sprintf(PattyBytesColor+"Peak bytes:%s[default]/min", peakBytes())
	}), "traffic", "peakBytes")

	g.Add(Random(5, func(_ []string) string {
		return fmt.Sprintf(PattyErrorColor+"Avg errs:%d[default]/min", avgErrs())
	}), "traffic", "avgErrs")
	g.Add(Random(5, func(_ []string) string {
		return fmt.Sprintf(PattyLinesColor+"Avg lines:%s[default]/min", avgLines())
	}), "traffic", "avgLines")
	g.Add(Random(5, func(_ []string) string {
		return fmt.Sprintf(PattyBytesColor+"Avg bytes:%s[default]/min", avgBytes())
	}), "traffic", "avgBytes")

	//g.Add(Random(5, func(_ []string) string {
	//	var mem runtime.MemStats
	//	runtime.ReadMemStats(&mem)
	//	return fmt.Sprintf("Heap:[palegreen]%s[default] / Allocs:[palegreen]%s[default]",
	//		formatBytes(int(mem.HeapAlloc)),
	//		formatBytes(int(mem.TotalAlloc)))
	//}))
	g.Add(Random(4, func(_ []string) string {
		return gradientText("PattyGraph", rainbowGradient)
	}))
	//g.Add(Random(10, func(_ []string) string {
	//	return fmt.Sprintf("LineCh: [palegreen]%d/%d[default]", len(lineCh), cap(lineCh))
	//}))
	//g.Add(Random(5, func(_ []string) string {
	//	var m runtime.MemStats
	//	runtime.ReadMemStats(&m)
	//	return fmt.Sprintf("NextGC target:[green]%s[default]",
	//		formatBytes64(m.NextGC),
	//	)
	//}))
	g.Add(Random(10, func(_ []string) string {
		cur := len(PattyGraph.wordsMatcher.peakWords)
		avg := PattyGraph.wordsMatcher.averagePeakWords()
		return fmt.Sprintf(toolFmt("Words #Peak/avg:%d/%.0f"), cur, avg)
	}), "interesting", "wordsPeak")
	g.Add(Random(10, func(_ []string) string {
		cur := len(PattyGraph.refsMatcher.peakWords)
		avg := PattyGraph.wordsMatcher.averagePeakWords()
		return fmt.Sprintf(toolFmt("Refs #Peak/avg:%d/%.0f"), cur, avg)
	}), "interesting", "refsPeak")
	g.Add(Random(15, func(_ []string) string {
		singles := 0
		total := len(PattyGraph.refsMatcher.wordFrequency)
		for _, stats := range PattyGraph.refsMatcher.wordFrequency {
			if stats.primeFlux == 1 {
				singles++
			}
		}
		percent := 0.0
		if total > 0 {
			percent = (float64(singles) / float64(total)) * 100
		}
		return fmt.Sprintf(toolFmt("1-Hit Refs:%s(%.0f%%)"),
			strings.TrimSpace(formatCounts(singles)),
			percent,
		)
	}), "interesting", "oneHitRefs")
	g.Add(Random(15, func(_ []string) string {
		singles := 0
		total := len(PattyGraph.wordsMatcher.wordFrequency)
		for _, stats := range PattyGraph.wordsMatcher.wordFrequency {
			if stats.primeFlux == 1 {
				singles++
			}
		}
		percent := 0.0
		if total > 0 {
			percent = (float64(singles) / float64(total)) * 100
		}
		return fmt.Sprintf(toolFmt("1-Hit Words:%s(%.0f%%)"),
			strings.TrimSpace(formatCounts(singles)),
			percent,
		)
	}), "interesting", "oneHitWords")

	g.Add(Random(15, func(_ []string) string {
		singles := 0
		total := len(PattyGraph.ipsMatcher.wordFrequency)
		for _, stats := range PattyGraph.ipsMatcher.wordFrequency {
			if stats.primeFlux == 1 {
				singles++
			}
		}
		percent := 0.0
		if total > 0 {
			percent = (float64(singles) / float64(total)) * 100
		}
		return fmt.Sprintf(ipFmt("1-Hit IPs:%s(%.0f%%)"),
			strings.TrimSpace(formatCounts(singles)),
			percent,
		)
	}), "interesting", "oneHitIPs")
	//g.Add(Repeating(15, func(_ []string) string {
	//	matchers := PattyGraph.matchers
	//	n := len(matchers)
	//
	//	// Require at least one eligible matcher
	//	if n <= 6 {
	//		return ""
	//	}
	//
	//	eligible := matchers[:n-6]
	//
	//	// Filter those with history
	//	var candidates []MatcherFacade
	//	for _, mf := range eligible {
	//		m := mf.asMatcher()
	//		if m != nil && len(m.history) > 0 {
	//			candidates = append(candidates, m)
	//		}
	//	}
	//	if len(candidates) == 0 {
	//		return ""
	//	}
	//
	//	// Pick one randomly
	//	selected := candidates[rand.Intn(len(candidates))]
	//	avg := averageIntSlice(selected.asMatcher().history)
	//	return fmt.Sprintf(wrapFmt("%s a≈%s/min", selected.asMatcher().color),
	//		selected.matcherName(), strings.TrimSpace(formatCounts(avg)))
	//}), "matcher", "#rnd")
	g.Add(Repeating(15, func(args []string) string {
		if args == nil || len(args) == 0 {
			return ""
		}
		matchers := PattyGraph.matchers
		n := len(matchers)
		name := args[0]

		// Require at least one eligible matcher
		if n <= 6 {
			return ""
		}

		eligible := matchers[:n-6]

		var selected MatcherFacade
		for _, mf := range eligible {
			m := mf.asMatcher()
			if m != nil && m.name == name {
				selected = mf
				break
			}
		}
		if selected == nil {
			return ""
		}
		m := selected.asMatcher()
		//avg := averageIntSlice(m.history)
		return m.pushStatsMsg()
		//return fmt.Sprintf(wrapFmt("%s a≈%s/min", m.color),
		//	selected.matcherName(), strings.TrimSpace(formatCounts(avg)))
	}), "matcher")

	g.Add(Repeating(50, func(args []string) string {
		matchers := PattyGraph.matchers
		n := len(matchers) - 6
		// Require at least one eligible matcher
		if n <= 0 {
			return ""
		}

		selected := matchers[matcherMarchCount]
		matcherMarchCount++
		if matcherMarchCount > n {
			matcherMarchCount = 0
		}

		if selected == nil {
			return ""
		}
		m := selected.asMatcher()
		if m == nil {
			return ""
		}
		//avg := averageIntSlice(m.history)
		return m.pushStatsMsg()
		//return fmt.Sprintf(wrapFmt("%s a≈%s/min", m.color),
		//	selected.matcherName(), strings.TrimSpace(formatCounts(avg)))
	}), "matcher", "march")

	//g.Add(Repeating(10, func(_ []string) string {
	//	var counts uint64
	//	//counts := make([]string, 0, len(historyTemplateReuse))
	//	for i := 0; i < len(historyTemplateReuse); i++ {
	//		//counts = append(counts, fmt.Sprintf("%d:%s", i, formatCountsUint64(historyTemplateReuse[i])))
	//		counts += historyTemplateReuse[i]
	//	}
	//	//return fmt.Sprintf(memFmt("hT reuse:%s"), strings.Join(counts, " "))
	//	return fmt.Sprintf(memFmt("hT reuse:%s"), formatCountsUint64(counts))
	//}), "mem", "htReuse")

	//g.Add(Repeating(15, func(_ []string) string {
	//	return fmt.Sprintf(memFmt("lSrcNew:%s"), formatCountsUint64(lsPoolNews))
	//}), "mem", "lsNew")
	//g.Add(Repeating(15, func(_ []string) string {
	//	return fmt.Sprintf(memFmt("lSrcGets:%s"), formatCountsUint64(lsPoolGets))
	//}), "mem", "lsGet")
	//g.Add(Repeating(15, func(_ []string) string {
	//	return fmt.Sprintf(memFmt("lSrcRets:%s"), formatCountsUint64(lsPoolReturns))
	//}), "mem", "lsRet")
	//g.Add(Repeating(15, func(_ []string) string {
	//	reuseRate := float64(lsPoolGets-lsPoolNews) / float64(lsPoolGets) * 100
	//	return fmt.Sprintf(memFmt("lsReuse:%.1f%%"), reuseRate)
	//}), "mem", "lsReuse")

	//g.Add(Repeating(10, func(_ []string) string {
	//	newCount := lsPoolNews
	//	getCount := lsPoolGets
	//
	//	if getCount == 0 {
	//		return ""
	//	}
	//
	//	allocRate := 100.0 * float64(newCount) / float64(getCount)
	//	return fmt.Sprintf(memFmt("lsAlloc rate:%s of:%s(%.1f%%)"),
	//		formatCountsUint64(newCount),
	//		formatCountsUint64(getCount),
	//		allocRate)
	//}), "mem", "lsAllocRate")

	//var lastGets, lastNews uint64

	//g.Add(Repeating(100, func(_ []string) string {
	//	gets := lsPoolGets - lastGets
	//	news := lsPoolNews - lastNews
	//
	//	lastGets = lsPoolGets
	//	lastNews = lsPoolNews
	//
	//	percent := 0.0
	//	if gets > 0 {
	//		percent = float64(gets-news) / float64(gets) * 100
	//	}
	//	return fmt.Sprintf(memFmt("lsReuseΔ:%.1f%%"), percent)
	//}))

	//g.Add(Repeating(10, func(_ []string) string {
	//	liveEstimate := lsPoolGets - lsPoolReturns
	//	return fmt.Sprintf(memFmt("lsLiveEst:%s"), formatCountsUint64(liveEstimate))
	//}))

	//g.Add(Repeating(100, func(_ []string) string {
	//	evaporated := lsPoolReturns - (lsPoolGets - lsPoolNews)
	//	return fmt.Sprintf(memFmt("lsEvapEst:%s"), formatCountsUint64(evaporated))
	//}))

	//g.Add(Repeating(100, func(_ []string) string {
	//	peak := lsPoolReturns - lsPoolGets + lsPoolNews
	//	return fmt.Sprintf(memFmt("lsPeakCap:%s"), formatCountsUint64(peak))
	//}))

	g.Add(Repeating(5, func(_ []string) string {
		return fmt.Sprintf(memFmt("wsNew:%s"), formatCountsUint64(poolNews))
	}), "mem", "wsNew")
	g.Add(Repeating(5, func(_ []string) string {
		return fmt.Sprintf(memFmt("wsRets:%s"), formatCountsUint64(poolReturns))
	}), "mem", "wsRets")
	g.Add(Repeating(5, func(_ []string) string {
		return fmt.Sprintf(memFmt("wsGets:%s"), formatCountsUint64(poolGets))
	}), "mem", "wsGets")

	//g.Add(Repeating(15, func(_ []string) string {
	//	return fmt.Sprintf(memFmt("rbGets:%s"), formatCountsUint64(rbGets))
	//}), "mem", "rbGets")
	//g.Add(Repeating(15, func(_ []string) string {
	//	return fmt.Sprintf(memFmt("rbNews:%s"), formatCountsUint64(rbNews))
	//}), "mem", "rbNews")
	//g.Add(Repeating(15, func(_ []string) string {
	//	return fmt.Sprintf(memFmt("rbPuts:%s"), formatCountsUint64(rbReturns))
	//}), "mem", "rbPuts")
	//g.Add(Repeating(15, func(_ []string) string {
	//	return fmt.Sprintf(memFmt("rbGets - rbNews:%s"), formatCountsUint64(rbGets-rbNews))
	//}), "mem", "rbDiff")
	//g.Add(Repeating(15, func(_ []string) string {
	//	return fmt.Sprintf(memFmt("rb n/g/r:%s/%s/%s"), formatCountsUint64(rbNews), formatCountsUint64(rbGets), formatCountsUint64(rbReturns))
	//}), "mem", "rbAll")
	//g.Add(Repeating(10, func(_ []string) string {
	//	measured := len(PattyGraph.wordsMatcher.wordFrequency) +
	//		len(PattyGraph.refsMatcher.wordFrequency) +
	//		len(PattyGraph.ipsMatcher.wordFrequency)
	//	return fmt.Sprintf(memFmt("wsMeasured:%s"), trimmedCounts(measured))
	//}))

	//g.Add(Repeating(10, func(_ []string) string {
	//	return fmt.Sprintf(memFmt("prefixRecycleCount:%s"), formatCounts(prefixRecycleCount))
	//}), "mem", "prc")

	g.Add(Repeating(5, func(_ []string) string {
		measured := len(PattyGraph.wordsMatcher.wordFrequency) +
			len(PattyGraph.refsMatcher.wordFrequency) +
			len(PattyGraph.ipsMatcher.wordFrequency)
		diff := int(poolGets-poolReturns) - measured
		if diff == 0 {
			return ""
		}
		return fmt.Sprintf(memFmt("wsEst-wsM:%s"), trimmedCounts(diff))
	}), "mem", "wsDiff")
	g.Add(Repeating(5, func(_ []string) string {
		reuseRate := float64(poolGets-poolNews) / float64(poolGets) * 100
		return fmt.Sprintf(memFmt("wsReuse:%.1f%%"), reuseRate)
	}), "mem", "wsReuse")
	g.Add(Repeating(5, func(_ []string) string {
		estimatedEvaporated := poolReturns - (poolGets - poolNews)
		return fmt.Sprintf(memFmt("wsEvapEst:%s"), formatCountsUint64(estimatedEvaporated))
	}), "mem", "wsEvap")

	g.Add(Repeating(5, func(_ []string) string {
		newCount := poolNews
		getCount := poolGets

		if getCount == 0 {
			return ""
		}

		allocRate := 100.0 * float64(newCount) / float64(getCount)
		return fmt.Sprintf(memFmt("wsAlloc rate:%s of:%s(%.1f%%)"),
			formatCountsUint64(newCount),
			formatCountsUint64(getCount),
			allocRate)
	}), "mem", "wsAllocRate")

	//g.Add(Repeating(10, func(_ []string) string {
	//	peakPool := poolReturns - poolGets + poolNews
	//	return fmt.Sprintf(memFmt("wsPeakCap:%s"), formatCountsUint64(peakPool))
	//}))

	//g.Add(Repeating(10, func(_ []string) string {
	//	//return fmt.Sprintf(memFmt("hT reuse:%s"), strings.Join(counts, " "))
	//	return fmt.Sprintf(memFmt("poolSize:%s"), formatCountsUint64(wordStatsPool.))
	//}))
	g.Add(Random(50, func(_ []string) string {
		type spikeInfo struct {
			name  string
			color string
			score float64
		}
		var bursts []spikeInfo

		// Skip last 6 matchers
		for i := 0; i < len(PattyGraph.matchers)-6; i++ {
			m := PattyGraph.matchers[i].asMatcher()
			b := m.spikiness()
			if b > 0 {
				bursts = append(bursts, spikeInfo{name: m.name, color: m.color, score: b})
			}
		}

		if len(bursts) == 0 {
			return ""
		}

		// Sort descending by spikiness
		sort.Slice(bursts, func(i, j int) bool {
			return bursts[i].score > bursts[j].score
		})

		// Limit to top 3
		if len(bursts) > 3 {
			bursts = bursts[:3]
		}

		// Format nicely
		var names []string
		for _, b := range bursts {
			names = append(names, fmt.Sprintf(wrapFmt("%sΔ%.0f", b.color), b.name, b.score))
		}

		return fmt.Sprintf("Most Volatile: %s", strings.Join(names, ", "))
	}), "most", "volatile")
	g.Add(Random(50, func(_ []string) string {
		type activeInfo struct {
			name    string
			color   string
			count   int
			current int
		}
		var top []activeInfo

		// Skip last 6 matchers
		for i := 0; i < len(PattyGraph.matchers)-6; i++ {
			m := PattyGraph.matchers[i].asMatcher()
			n := m.lastIntervalCount
			c := m.intervalCount
			if n > 0 {
				top = append(top, activeInfo{name: m.name, color: m.color, count: n, current: c})
			}
		}

		if len(top) == 0 {
			return ""
		}

		// Sort descending by recent count
		sort.Slice(top, func(i, j int) bool {
			return (top[i].count + top[i].current) > (top[j].count + top[j].current)
		})

		// Limit to top 3
		if len(top) > 3 {
			top = top[:3]
		}

		// Format nicely
		var names []string
		for _, a := range top {
			names = append(names, fmt.Sprintf(wrapFmt("%s %s:%s", a.color), a.name, trimmedCounts(a.current), trimmedCounts(a.count)))
		}

		return fmt.Sprintf("Most Active: %s", strings.Join(names, ", "))
	}), "most", "active")

	g.Add(Random(20, func(_ []string) string {
		if PattyGraph.ipsMatcher.wordStatsCreated < 100 {
			return ""
		}
		ipsRate := float64(PattyGraph.ipsMatcher.wordStatsCreated) / float64(PattyGraph.intervalLines)
		wordsRate := float64(PattyGraph.wordsMatcher.wordStatsCreated) / float64(PattyGraph.intervalLines)
		refsRate := float64(PattyGraph.refsMatcher.wordStatsCreated) / float64(PattyGraph.intervalLines)
		return fmt.Sprintf(toolFmt("ΔNew w:%.0f%% r:%.0f%% i:%.0f%%"),
			wordsRate*100, refsRate*100, ipsRate*100)
	}), "mem", "newEntries")

	//g.Add(Random(10, func(_ []string) string {
	//	return fmt.Sprintf(toolFmt("repop w:%d r:%d i:%d"),
	//		PattyGraph.wordsMatcher.wordStatsRepopMetric.Latest(),
	//		PattyGraph.refsMatcher.wordStatsRepopMetric.Latest(),
	//		PattyGraph.ipsMatcher.wordStatsRepopMetric.Latest(),
	//	)
	//}), "mem", "repopEntries")

	tips := 1
	nextTip := func() string {
		s := strconv.Itoa(tips)
		tips++
		return s
	}

	g.Add(Random(1, func(_ []string) string { return tipText("Config file lines are also inline commands") }), "tip", nextTip())
	g.Add(Random(1, func(_ []string) string { return tipText("ctrl-h opens Quick Help") }), "tip", nextTip())
	g.Add(Random(1, func(_ []string) string { return tipText("ctrl-g Generate config") }), "tip", nextTip())
	g.Add(Random(1, func(_ []string) string { return tipText("ctrl-s Save screen") }), "tip", nextTip())
	g.Add(Random(1, func(_ []string) string { return tipText("ctrl-m create matcher from selection") }), "tip", nextTip())
	g.Add(Random(1, func(_ []string) string { return tipText("Tab cycles secondary info display") }), "tip", nextTip())
	g.Add(Random(1, func(_ []string) string { return tipText("</> adjusts Grace period") }), "tip", nextTip())
	// leave this out until brackets can be safely printed
	// g.Add(Random(1, func(_ []string) string { return tipText("[/] adjusts Secondary graphing window") }))
	g.Add(Random(1, func(_ []string) string { return tipText("ctrl-up/down adjusts Scale factor") }), "tip", nextTip())
	g.Add(Random(1, func(_ []string) string { return tipText("ctrl-left/right adjusts Push factor") }), "tip", nextTip())
	g.Add(Random(1, func(_ []string) string { return tipText("f/F 5/10 pause Interesting display updates") }), "tip", nextTip())
	g.Add(Random(1, func(_ []string) string { return tipText("'x' toggles expert status") }), "tip", nextTip())
	g.Add(Random(1, func(_ []string) string { return tipText("'X' toggles ticker display") }), "tip", nextTip())
	g.Add(Random(1, func(_ []string) string { return tipText("'D' to move selected matcher Down") }), "tip", nextTip())
	g.Add(Random(1, func(_ []string) string { return tipText("'U' to move selected matcher Up") }), "tip", nextTip())
	g.Add(Random(1, func(_ []string) string { return tipText("ctrl-p purges PeakWords if no matcher selected") }), "tip", nextTip())
	g.Add(Random(1, func(_ []string) string { return tipText("ctrl-d deletes selected matcher") }), "tip", nextTip())
	g.Add(Random(1, func(_ []string) string { return tipText("ctrl-d on Bots disables auto-add") }), "tip", nextTip())
	g.Add(Random(1, func(_ []string) string { return tipText("Click the Ticker to show Factoid history") }), "tip", nextTip())

	return g
}
func memFmt(format string) string      { return wrapFmt(format, memGcColor) }
func toolFmt(format string) string     { return wrapFmt(format, toolColor) }
func internalFmt(format string) string { return wrapFmt(format, internalsColor) }
func ipFmt(format string) string       { return wrapFmt(format, ipColor) }

func tipText(tip string) string {
	return "[blue]Tip:[default] " + tip
}

// Bottom-panel modes share the lower-left TUI surface between matcher detail,
// factoid history, and quick help. The selected ticker background follows the
// factoid mode even while quick help is temporarily shown, so ctrl-h can restore
// the previous operator context.
type bottomPanelMode int

const (
	bottomPanelMatchers bottomPanelMode = iota
	bottomPanelFactoids
	bottomPanelHelp
)

var (
	bottomPanelCurrent       = bottomPanelMatchers
	bottomPanelReturnMode    = bottomPanelMatchers
	showMetricsPanelContents bool // true when the bottom-left panel is showing factoids
	tickerPreamble           = defaultTickerBg
)

func syncBottomPanelState() {
	showMetricsPanelContents = bottomPanelCurrent == bottomPanelFactoids
	if bottomPanelCurrent == bottomPanelFactoids ||
		(bottomPanelCurrent == bottomPanelHelp && bottomPanelReturnMode == bottomPanelFactoids) {
		tickerPreamble = selectedTickerBg
	} else {
		tickerPreamble = defaultTickerBg
	}
}

func togglePreamble() {
	if bottomPanelCurrent == bottomPanelFactoids {
		bottomPanelCurrent = bottomPanelMatchers
	} else {
		bottomPanelCurrent = bottomPanelFactoids
	}
	if bottomPanelCurrent != bottomPanelHelp {
		bottomPanelReturnMode = bottomPanelCurrent
	}
	syncBottomPanelState()
}

func toggleHelpPanel() {
	if bottomPanelCurrent == bottomPanelHelp {
		bottomPanelCurrent = bottomPanelReturnMode
	} else {
		bottomPanelReturnMode = bottomPanelCurrent
		bottomPanelCurrent = bottomPanelHelp
	}
	syncBottomPanelState()
}

func currentTickerText() string {
	const graphWidth = 100
	const scrollSpeedChars = 3
	const bufferHeadroom = 3

	// Fill buffer with more factoids as needed
	for visibleRuneWidth(tickerBuffer)-tickerVisibleOffset < graphWidth+bufferHeadroom {
		tickerBuffer += getWrappedFactoid()
	}

	// Extract the visible slice
	visible, _ := sliceFromVisibleOffset(tickerBuffer, tickerVisibleOffset, graphWidth, tickerPreamble)

	// Advance scroll position
	tickerVisibleOffset += scrollSpeedChars
	return visible
}

// quickHelpPanelContents is intentionally terse. It is an in-TUI memory aid, not
// a replacement for --help.
func quickHelpPanelContents() string {
	return `[white:black]Quick Help
[default:black] KEYS
  [white:black]^h[default:black] help       [white:black]q[default:black] quit
  [white:black]x[default:black] expert      [white:black]X[default:black] ticker
  [white:black]^g[default:black] config     [white:black]^s[default:black] splat
  [white:black]esc[default:black] clear selection
  [white:black]tab[default:black] cycle info view
  [white:black]</>[default:black] grace     [white:black]{/}[default:black] flux
  [white:black]^left/^right[default:black]  push
  [white:black]^up/^down[default:black]     scale
  [white:black]^m[default:black] item top/Bots pop
  [white:black]^b[default:black] item above Bots
  [white:black]^n[default:black] item under Bots
  [white:black]^d[default:black] delete/Bots off
  [white:black]U/D[default:black] move matcher
  [white:black]^p[default:black] clear matcher/Peak
  [white:black][/][default:black] mini spark range
  [white:black]f/F[default:black] freeze tui 5/10

 MOUSE
  W/R/IP    entry select
  spark     inspect
  matcher   select
  ^matcher  detail level
  ticker    fact panel
  ^ticker   hide
`
}

// Factoid panel rendering reuses this builder because the panel is redrawn often.
var panelBuilder = strings.Builder{}

func metricPanelContents() string {
	panelBuilder.Reset()
	panelBuilder.WriteString("  ")
	panelBuilder.WriteString(gradientText(PattyGraphName, rainbowGradient))
	panelBuilder.WriteString(" ")
	panelBuilder.WriteString(gradientText(PattyGraphVersion, rainbowGradient))
	panelBuilder.WriteString("\n")
	sort.Slice(facts.facts, func(i, j int) bool {
		return facts.facts[i].LastSeen > facts.facts[j].LastSeen
	})

	panelBuilder.WriteString("[default:black]")
	panelBuilder.WriteString(strings.Join(factoidHistory, "\n"))
	//for i, fact := range facts.facts {
	//	if fact.probability >= 5 && fact.cache != "" {
	//		panelBuilder.WriteString(fact.cache)
	//		panelBuilder.WriteString("\n")
	//	}
	//	if i > 50 {
	//		break
	//	}
	//}
	return panelBuilder.String()
}

var fmtTokenRe = regexp.MustCompile(`%[0-9.]*[sdf]`)

// wrapFmt takes a format string and a color string, and returns a new format
// string where all %s, %d, and %f are wrapped in the given color and "[default]".
//
//go:format wrapFmt 1
func wrapFmt(format, color string) string {
	return fmtTokenRe.ReplaceAllStringFunc(format, func(token string) string {
		return color + token + "[default]"
	})
}
func averageIntSlice(data []int) int {
	if len(data) == 0 {
		return 0
	}
	sum := 0
	for _, v := range data {
		sum += v
	}
	return sum / len(data)
}

func avgErrs() int {
	return averageIntSlice(PattyGraph.errsMatcher.history)
}

func avgBytes() string {
	return formatBytes(averageIntSlice(PattyGraph.bytesMatcher.history))
}

func avgLines() string {
	return formatCounts(averageIntSlice(PattyGraph.linesMatcher.history))
}

var rainbowGradient = []string{"firebrick", "orangered", PattyOrange, "green", "blue", "indigo", "violet"}
