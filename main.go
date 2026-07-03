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

var PattyGraph *Monitor

var InterestingWordListSize = 100 // todo This needs to be settable
var colorIndex = 0
var pattyPushFactor = pattyPushFactorDefault
var pattyScaleFactor = pattyScaleFactorDefault
var pattyGracePeriod = pattyGracePeriodDefault
var machineDisplayName string

var forceZeroStart bool
var expertMode bool
var generateJsonSparks bool
var generateSidecarJSONL bool
var enableControlFile bool

// metrics
var logLoadDuration time.Duration
var logLoadLinecount uint64
var logLoadIntervalCount int
var logLoadGCCost int64

// PattyGraph is logically, hopelessly and naively "single threaded"
// (ignoring support threads):
//
//	startFileMonitoring()        // tails the access log data plane
//	startControlFileMonitoring() // tails the control file command plane
//	startUI()                    // launches display
//
// There's a match thread that's running off of access log reading, a control thread that's
// reading inline commands from pattyControl.log, and the display thread that's queueing
// display events into tview's display cycle. These active processes take turns being
// active on the same lock. The result is within the
// PattyGraph codebase, it can be naively single threaded. This wasn't the initial
// idea but even early coordination at top matcher levels was seeing horrible timing
// issues. Turns out, when everything is lean and fast enough, a naive approach can work.
// If this was ever an issue, batching would be the way to go, but its never been an issue.
// The display thread shouldn't be altering the underlying model of counts per interval
// calculations but it does perform the push(). It may be enough to lock the push() if
// parallelism is ever wanted.
//
// Because of the single threadedness of everything, and I'm not overly concerned about
// space, I have a number of slice caches located by area of need. If there were threading
// issues, these would be better in a reusable Pool.
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

	// parseArgs and NewMonitor are a somewhat combined application init, cli reading, default value enforcement, etc.
	// CLI overrides were going to be problematic to pull out so instead they're enforced twice, here and below.
	// It was just cleaner and easier to not disentangle it all and repeat the settings enforcement later with the
	// pflag visitor pattern so only flags on the command line are processed the second time.
	mConf := parseArgs()
	PattyGraph = NewMonitor(mConf)
	botsIndex = botsMatcherIndex()

	cpuName, heapName, allocName := "", "", ""
	// symbol table drop flag in ./compile.sh may need to be adjusted
	/****** for profiling ******/
	//timeKey := time.Now().Format("0102_1504")
	//// CPU
	//cpuName = fmt.Sprintf("cpu_pattyGraph_%s.prof", timeKey)
	//f, _ := os.Create(cpuName)
	//pprof.StartCPUProfile(f)     // Start CPU profiling
	//defer pprof.StopCPUProfile() // Stop profiling and write to file
	//
	////Heap & Alloc
	//heapName = fmt.Sprintf("heap_pattyGraph_%s.pb.gz", timeKey)
	//allocName = fmt.Sprintf("alloc_pattyGraph_%s.pb.gz", timeKey)
	//defer func() {
	//	f, _ := os.Create(heapName)
	//	pprof.Lookup("heap").WriteTo(f, 0)
	//
	//	f2, _ := os.Create(allocName)
	//	pprof.Lookup("allocs").WriteTo(f2, 0)
	//}()
	/****** end profiling ******/

	if cpuName != "" {
		fmt.Printf("CPU profile written to: %s\n", cpuName)
	}
	if heapName != "" {
		fmt.Printf("Heap profile written to: %s\n", heapName)
	}
	if allocName != "" {
		fmt.Printf("Alloc profile written to: %s\n", allocName)
	}
	PattyGraph.playConfigFile(mConf.builtinConfFile)
	enforceCliFlags()
	startControlFileMonitoring()
	// Sidecar stream marker: write this before startup replay so AI consumers
	// can correlate both ghost-run preload intervals and live TUI intervals
	// under one session_id. The --json CLI flag gates this stream.
	if generateSidecarJSONL {
		if err := PattyGraph.WriteSidecarSessionStartJSONL(""); err != nil {
			log.Printf("sidecar session start write failed: %v", err)
		}
	}
	beforeLoadTime := time.Now()
	preloadRecentMinutes()
	logLoadDuration = time.Since(beforeLoadTime)
	logLoadLinecount = PattyGraph.totalLines
	logLoadIntervalCount = PattyGraph.intervalsCompleted
	var stats debug.GCStats
	debug.ReadGCStats(&stats)
	logLoadGCCost = stats.NumGC

	startFileMonitoring() // tails the access log data plane
	doRandom = true
	startUI() // Feeds tview's display cycle
}

func enforceCliFlags() {
	// CLI Overrides happens here.
	// This is terrible but I just didn't want to redo everything else around this for now
	// The goodness this provides far outweighs the distaste of repeating config logic in 3 places
	// TODO: consolidate with other places that do this and reuse inline command interpretation instead of
	//       going straight to setting the values.
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
		case "sparkout":
			generateJsonSparks = f.Value.String() == "true"
		case "json":
			generateSidecarJSONL = f.Value.String() == "true"
		case "control":
			enableControlFile = f.Value.String() == "true"
		case "color-index":
			value, _ := strconv.Atoi(f.Value.String())
			if value != colorIndex {
				colorIndex = value
				pushFactNow("settings.colorIndex", nil)
			}
		case "save-dir":
			PattyGraph.pattyConfig.saveDirOriginal = f.Value.String()
			PattyGraph.pattyConfig.saveDir = expandUser(f.Value.String())
		}
	})
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
			var sidecarSnapshot SidecarInterval
			if generateSidecarJSONL {
				// Startup sidecar interval: preload replays completed historical
				// minutes without the TUI timer, but the same push boundary
				// semantics apply. Capture before push() resets interval state.
				sidecarSnapshot = PattyGraph.SidecarSnapshotBeforePush()
			}
			push() // Push only if NOT the last minute
			if generateSidecarJSONL {
				if err := PattyGraph.WriteSidecarJSONL(sidecarSnapshot, ""); err != nil {
					log.Printf("PattyLog preload jsonl write failed: %v", err)
				}
			}
			PattyGraph.writePendingAlertTransitionsJSONL()
			PattyGraph.clearPendingAlertTransitions()
		} else {
			stopTime := time.Now().Second()
			// if seconds went from say 57...2 we need to do a push to catch up.
			if startTime > stopTime {
				logicalCycles += 60
				var sidecarSnapshot SidecarInterval
				if generateSidecarJSONL {
					// Startup sidecar interval: wall-clock rollover means the
					// partial current minute should also be committed as a completed
					// interval before live tailing begins.
					sidecarSnapshot = PattyGraph.SidecarSnapshotBeforePush()
				}
				push()
				if generateSidecarJSONL {
					if err := PattyGraph.WriteSidecarJSONL(sidecarSnapshot, ""); err != nil {
						log.Printf("PattyLog preload jsonl write failed: %v", err)
					}
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
