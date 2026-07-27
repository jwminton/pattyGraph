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
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/pflag"
)

// PattyGraph reads an NGINX access log as a live stream of traffic evidence.
// In particular, PattyGraph is written like a high performance enterprise
// levevl server that returns no response. Each logline (request) enters an
// ordered pipeline of matchers, bot handling, token retention, IP/referrer/URI
// aggregation, alert checks, and time-windowed history. A line can claim
// attention, reinforce an existing pattern, expose a new token, update a
// retained counter, or pass through without becoming part of the operational
// surface.
//
// A useful shorthand for the design is grep with a sliding window, a memory,
// and a garbage collector. Matching is only the first step. Recency, repetition,
// contrast, matcher order, and retained state decide which facts survive long
// enough to become signal.
//
// The display and JSONL output are projections of the same running model. The
// terminal UI gives an engineer a live surface for triage. The JSONL sidecar
// gives AI and other machine consumers compact context without asking them to
// read the raw log stream.
//
// Measures in the model need useful dynamic ranges. A token, counter, or matcher
// that is always saturated, always empty, or present in ordinary background
// traffic adds little distinguishing value. PattyGraph favors signals that can
// separate one traffic shape from another: a referrer shift, a URI fragment, an
// IP cluster, a bot boundary crossing, a matcher spike, or a token that appears
// where it normally does not. No mystery gets solved by tracking "Mozilla" in
// NGINX user agents.
//
// The process is intentionally organized around one shared Monitor:
//
//     startFileMonitoring()        // access-log data plane
//     startControlFileMonitoring() // control-file command plane
//     startUI()                    // human display plane
//
// Matcher order, retained history, click selection, factoids, alerts, sidecar
// snapshots, and time pressure are all views of the same running traffic shape.
// Keeping them close to one shared model keeps the hot path fast, and the runtime
// behavior coherent.

var InterestingWordListSize = 100 // todo This needs to be settable
var colorIndex = 0
var pattyPushFactor = pattyPushFactorDefault
var pattyScaleFactor = pattyScaleFactorDefault
var pattyGracePeriod = pattyGracePeriodDefault
var machineDisplayName string

var forceZeroStart bool
var expertMode bool

// JSONL emission and source-example enrichment are independent runtime choices.
var generateSidecarJSONL bool
var includeSidecarSourceExamples bool
var enableControlFile bool

// metrics
var logLoadDuration time.Duration
var logLoadLinecount uint64
var logLoadIntervalCount int
var logLoadGCCost int64

func main() {
	// This should already be the default but lets be sure since terminal output on a panic is lost
	log.SetOutput(os.Stderr)
	defer handlePanicToFile()
	//defer func() {
	//	if r := recover(); r != nil {
	//		log.Printf("Panic: %v\n", r)
	//		log.Printf("Stack trace:\n%s\n", debug.Stack())
	//		log.Println("pattyGraph panicked!")
	//	}
	//}()

	// Startup intentionally has three configuration passes:
	//   1. parse CLI/defaults into a MonitorConfig
	//   2. replay saved config as inline commands
	//   3. reapply explicit CLI flags as final authority
	//
	// This looks heavier than direct pflag usage, but it keeps startup config,
	// generated config, live control-file commands, and inline log commands on the
	// same command language.
	mConf := parseArgs()
	PattyGraph = NewMonitor(mConf)
	botsIndex = botsMatcherIndex()

	// Hot-path profiling is kept commented so runtime/pprof is not imported in
	// normal builds. Uncomment the helper below and this call site when doing
	// deep preload/matching/TUI startup profiling.
	//cpuName, heapName, allocName, stopProfiling := maybeStartHotPathProfiling()
	//defer stopProfiling()
	//fmt.Printf("Profiles: cpu=%s heap=%s alloc=%s\n", cpuName, heapName, allocName)

	PattyGraph.playConfigFile(mConf.builtinConfFile)
	enforceCliFlags()
	if err := ensureSaveDir(PattyGraph.pattyConfig); err != nil {
		log.Fatalf("Failed to create save-dir %s: %v", PattyGraph.pattyConfig.saveDir, err)
	}
	startControlFileMonitoring()
	// Sidecar stream marker: write this before startup replay so AI consumers
	// can correlate both ghost-run preload intervals and live TUI intervals
	// under one session_id. The --json CLI flag gates this stream.
	if generateSidecarJSONL {
		recordSidecarWriteResult("session_start", PattyGraph.WriteSidecarSessionStartJSONL(""))
	}
	// Preload is part of the product behavior, not a warmup hack. Reading the
	// recent tail of a large access log gives the TUI immediate traffic shape:
	// matcher history, interesting-key rankings, peak entries, and bot competition
	// are already populated before the operator sees the first screen.
	beforeLoadTime := time.Now()
	preloadRecentMinutes()
	loadDuration := time.Since(beforeLoadTime)
	var stats debug.GCStats
	debug.ReadGCStats(&stats)
	// Control-file fact requests can arrive during preload. Publish the completed
	// startup measurements together so init.* facts never observe a partial set.
	mu.Lock()
	logLoadDuration = loadDuration
	logLoadLinecount = PattyGraph.totalLines
	logLoadIntervalCount = PattyGraph.intervalsCompleted
	logLoadGCCost = stats.NumGC
	mu.Unlock()

	startFileMonitoring() // tails the access log data plane
	doRandomFact = true
	startUI() // Feeds tview's display cycle
}

func enforceCliFlags() {
	// Reapply explicitly provided CLI flags after config replay so command-line
	// choices remain authoritative. This repeats some parseArgs behavior on
	// purpose: config files are serialized as inline commands and replayed before
	// startup, but direct CLI input should still be the final startup override.
	pflag.Visit(func(f *pflag.Flag) {
		switch f.Name {
		case "read":
			PattyGraph.pattyConfig.mbToRead, _ = strconv.Atoi(f.Value.String())
		case "grace":
			value, _ := strconv.Atoi(f.Value.String())
			if value != pattyGracePeriod {
				pattyGracePeriod = value
				pushFactNow("settings.grace", nil)
			}
		case "flux":
			value, _ := strconv.Atoi(f.Value.String())
			if value != fluxDepth {
				fluxDepth = value
				pushFactNow("settings.flux", nil)
			}
		case "push":
			value, _ := strconv.Atoi(f.Value.String())
			if value != pattyPushFactor {
				pattyPushFactor = value
				pushFactNow("settings.push", nil)
			}
		case "scale":
			value, _ := strconv.ParseFloat(f.Value.String(), 64)
			if value != pattyScaleFactor {
				pattyScaleFactor = value
				pushFactNow("settings.scale", nil)
			}
		case "title":
			machineDisplayName = f.Value.String()
		case "expert":
			expertMode = f.Value.String() == "true"
		case "zero":
			forceZeroStart = f.Value.String() == "true"
		case "json":
			generateSidecarJSONL = f.Value.String() == "true"
		case "json-sources":
			includeSidecarSourceExamples = f.Value.String() == "true"
		case "json-file":
			PattyGraph.pattyConfig.setJSONFile(f.Value.String())
			generateSidecarJSONL = PattyGraph.pattyConfig.jsonFile != ""
		case "control":
			enableControlFile = f.Value.String() == "true"
		case "control-file":
			PattyGraph.pattyConfig.setControlFile(f.Value.String())
			enableControlFile = PattyGraph.pattyConfig.controlFile != ""
		case "color-index":
			value, _ := strconv.Atoi(f.Value.String())
			if value != colorIndex {
				colorIndex = value
				pushFactNow("settings.color-index", nil)
			}
		case "save-dir":
			PattyGraph.pattyConfig.setSaveDir(f.Value.String())
		}
	})
	if PattyGraph.pattyConfig.jsonFile != "" {
		generateSidecarJSONL = true
	}
	if PattyGraph.pattyConfig.controlFile != "" {
		enableControlFile = true
	}
}
func dumpFacts() {
	// Collect and sort fact names
	names := make([]string, 0, len(factoidByName))
	for _, fact := range factoidByName {
		names = append(names, fact.Name)
	}
	sort.Strings(names)

	const factHelpColumns = 4
	const factHelpColumnWidth = 24

	// Print in a fixed-column layout, left-aligned
	for i, name := range names {
		fmt.Printf("%-*s", factHelpColumnWidth, name)
		if i%factHelpColumns == factHelpColumns-1 || i == len(names)-1 {
			fmt.Println()
		}
	}
}

func dumpColors() {
	for i, color := range AutobotColors {
		clean := strings.Trim(color, "[]")
		if i%5 == 0 {
			fmt.Printf("%2d: ", i)
		}
		fmt.Printf("%-18s", clean)
		if i%5 == 4 || i == len(AutobotColors)-1 {
			fmt.Println()
		}
	}
}

func preloadRecentMinutes() error {
	if PattyGraph.pattyConfig.mbToRead == 0 {
		return nil
	}
	groups, err := groupLinesByMinuteInMb(PattyGraph.filePath, PattyGraph.pattyConfig.mbToRead)
	if err != nil {
		return err
	}

	// Extract and sort the minutes chronologically
	var minutes []MinuteGroup
	for _, minute := range groups {
		minutes = append(minutes, minute)
	}
	startTime := time.Now().Second()
	sort.Slice(minutes, func(i, j int) bool {
		return minutes[i].Timestamp.Before(minutes[j].Timestamp)
	})
	// Replay minute-by-minute
	for i, minute := range minutes {
		if i == 0 {
			// first group is always funky since we landed in the middle of a rando minute
			continue
		}
		// Order here is at the granularity of a minute. These might be out of order within the minute and that's ok
		for _, line := range minute.Lines {
			match(line) // Match each logLine in the minute
		}
		// Don't push the last minute, we're still probably in it,
		// It's ok if logicalCycles is off bc of this gap.
		if i != len(minutes)-1 || forceZeroStart {
			logicalCycles += 60
			prePush()
			var sidecarSnapshot SidecarInterval
			if generateSidecarJSONL {
				// Startup sidecar interval: preload replays completed historical
				// minutes without the TUI timer, but the same push boundary
				// semantics apply. Capture before push() resets interval state.
				sidecarSnapshot = PattyGraph.SidecarSnapshotBeforePush()
			}
			push() // Push only if NOT the last minute
			if generateSidecarJSONL {
				recordSidecarWriteResult("preload interval", PattyGraph.WriteSidecarJSONL(sidecarSnapshot, ""))
			}
			PattyGraph.writePendingAlertTransitionsJSONL()
			PattyGraph.clearPendingAlertTransitions()
		} else {
			stopTime := time.Now().Second()
			// if seconds went from say 57...2 we need to do a push to catch up.
			if startTime > stopTime {
				logicalCycles += 60
				prePush()
				var sidecarSnapshot SidecarInterval
				if generateSidecarJSONL {
					// Startup sidecar interval: wall-clock rollover means the
					// partial current minute should also be committed as a completed
					// interval before live tailing begins.
					sidecarSnapshot = PattyGraph.SidecarSnapshotBeforePush()
				}
				push()
				if generateSidecarJSONL {
					recordSidecarWriteResult("preload interval", PattyGraph.WriteSidecarJSONL(sidecarSnapshot, ""))
				}
				PattyGraph.writePendingAlertTransitionsJSONL()
				PattyGraph.clearPendingAlertTransitions()
			}
		}
	}
	return nil
}

var startBytesRead = 0

func groupLinesByMinuteInMb(filePath string, mbToRead int) ([]MinuteGroup, error) {
	const avgLineSize = 200              // adjust based on real logs if needed
	maxReadSize := int64(mbToRead) << 20 // MB to bytes

	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}

	size := stat.Size()
	offset := int64(0)
	if size > maxReadSize {
		offset = size - maxReadSize
	}
	startBytesRead = int(size - offset)
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(f)
	// Increase buffer if log lines are long
	const maxCapacity = 1024 * 1024 // 1MB per line max (tweak if needed)
	buf := make([]byte, 64*1024)    // start with 64KB buffer
	scanner.Buffer(buf, maxCapacity)

	// Pre-allocate map with rough estimate
	estimatedLines := int(maxReadSize / avgLineSize)
	estimatedMinutes := estimatedLines / 60 // very rough

	groupMap := make(map[time.Time][]string, estimatedMinutes)

	for scanner.Scan() {
		line := scanner.Text()
		ts, err := parseNginxTimeFast(line)
		if err != nil {
			continue
		}
		minute := ts.Truncate(time.Minute)
		groupMap[minute] = append(groupMap[minute], line)
	}

	// Convert map to sorted slice
	minuteGroups := make([]MinuteGroup, 0, len(groupMap))
	for k, v := range groupMap {
		minuteGroups = append(minuteGroups, MinuteGroup{Timestamp: k, Lines: v})
	}
	sort.Slice(minuteGroups, func(i, j int) bool {
		return minuteGroups[i].Timestamp.Before(minuteGroups[j].Timestamp)
	})

	return minuteGroups, scanner.Err()
}

type MinuteGroup struct {
	Timestamp time.Time
	Lines     []string
}

// Hot-path profiling helper.
//
// To use:
//  1. Add "runtime/pprof" to the imports.
//  2. Uncomment this helper.
//  3. Uncomment the maybeStartHotPathProfiling call near the top of main().
//
// Keep this close to runtime startup: hot-path profiling is most useful when it
// captures preload, matching, push, and TUI startup behavior together. The
// symbol table drop flag in compile.sh may need adjustment before interpreting
// profiles.
//
//func maybeStartHotPathProfiling() (cpuName string, heapName string, allocName string, stop func()) {
//	timeKey := time.Now().Format("0102_1504")
//
//	cpuName = fmt.Sprintf("cpu_pattyGraph_%s.prof", timeKey)
//	cpuFile, err := os.Create(cpuName)
//	if err != nil {
//		log.Printf("CPU profile create failed: %v", err)
//		cpuName = ""
//	} else if err := pprof.StartCPUProfile(cpuFile); err != nil {
//		log.Printf("CPU profile start failed: %v", err)
//		cpuFile.Close()
//		cpuName = ""
//	}
//
//	heapName = fmt.Sprintf("heap_pattyGraph_%s.pb.gz", timeKey)
//	allocName = fmt.Sprintf("alloc_pattyGraph_%s.pb.gz", timeKey)
//
//	return cpuName, heapName, allocName, func() {
//		if cpuName != "" {
//			pprof.StopCPUProfile()
//			cpuFile.Close()
//		}
//
//		if heapName != "" {
//			if f, err := os.Create(heapName); err == nil {
//				pprof.Lookup("heap").WriteTo(f, 0)
//				f.Close()
//			} else {
//				log.Printf("heap profile write failed: %v", err)
//			}
//		}
//
//		if allocName != "" {
//			if f, err := os.Create(allocName); err == nil {
//				pprof.Lookup("allocs").WriteTo(f, 0)
//				f.Close()
//			} else {
//				log.Printf("alloc profile write failed: %v", err)
//			}
//		}
//	}
//}

func handlePanicToFile() {
	saveDir := PattyGraph.pattyConfig.saveDir
	if r := recover(); r != nil {
		filename := newTimestampedFilename("panic_", ".txt")
		fullPath := filepath.Join(saveDir, filename)

		stack := cleanStackTrace("pattyGraph")

		f, err := os.Create(fullPath)
		if err != nil {
			// If file creation fails, fallback to stderr
			log.SetOutput(os.Stderr)
			log.Printf("Panic: %v\n", r)
			log.Printf("Stack trace:\n%s\n", debug.Stack())
			log.Printf(stack)
			log.Printf("Failed to write panic log to %s: %v", fullPath, err)
			return
		}
		defer f.Close()

		fmt.Fprintf(f, "Panic: %v\n", r)
		fmt.Fprintf(f, "\nTimestamp: %s\n", time.Now().Format(time.RFC3339))
		fmt.Fprintf(f, "Stack trace:\n%s\n", debug.Stack())
		fmt.Fprint(f, stack)
		fmt.Fprintf(f, "SaveDir: %s\n", saveDir)

		log.SetOutput(os.Stderr)
		log.Printf("pattyGraph panicked! Full log written to %s\n", fullPath)
		log.Printf("Panic: %v\n", r)
		log.Printf("Stack trace:\n%s\n", debug.Stack())
		log.Printf(stack)
	}
}

func cleanStackTrace(includePrefix string) string {
	stack := debug.Stack()
	lines := strings.Split(string(stack), "\n")

	var out []string
	out = append(out, "Stack trace (filtered):", "")

frameLoop:
	for i := 0; i < len(lines)-1; i += 2 {
		funcLine := strings.TrimSpace(lines[i])
		fileLine := strings.TrimSpace(lines[i+1])

		// Skip blank or malformed frames
		if funcLine == "" || fileLine == "" {
			continue
		}

		// Exclude noise: standard lib, runtime, tview, etc.
		if strings.Contains(funcLine, "/runtime/") ||
			strings.Contains(funcLine, "debug/stack.go") ||
			strings.Contains(funcLine, "github.com/rivo/tview") {
			continue frameLoop
		}

		// Match only your project's code (e.g., pattyGraph)
		if includePrefix != "" && !strings.Contains(funcLine, includePrefix) {
			continue frameLoop
		}

		// Trim full file path down to just filename:line
		shortFile := funcLine
		if idx := strings.LastIndex(funcLine, "/"); idx != -1 {
			shortFile = funcLine[idx+1:]
		}

		out = append(out, fmt.Sprintf("• %15s:%s", shortFile, fileLine))
	}

	return strings.Join(out, "\n")
}
