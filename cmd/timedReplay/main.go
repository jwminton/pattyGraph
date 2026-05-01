package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

// LogEntry represents a single log line with a timestamp
type LogEntry struct {
	timestamp time.Time
	line      string
}

// parseLogLine parses a log line, extracting the timestamp and the full line content
func parseLogLine(line string) (LogEntry, error) {
	// Assuming the timestamp is at the beginning of each line in the format "YYYY-MM-DDTHH:MM:SS"
	fields := strings.Fields(line)
	if len(fields) < 1 {
		return LogEntry{}, fmt.Errorf("invalid log line format")
	}
	timestampStr := fields[3]
	timestamp, err := time.Parse("[02/Jan/2006:15:04:05", timestampStr)
	//timestamp, err := time.Parse("2006-01-02T15:04:05", timestampStr)
	if err != nil {
		return LogEntry{}, err
	}
	return LogEntry{timestamp: timestamp, line: line}, nil
}

// replayLogStream reads, parses, and replays the log file in grouped intervals
func replayLogStream(filename string, speedMultiplier float64) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	var start, end *time.Time
	var lastMinute = -1
	scanner := bufio.NewScanner(file)
	var intervalStart time.Time
	var batch []LogEntry

	for scanner.Scan() {
		line := scanner.Text()
		entry, err := parseLogLine(line)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Skipping line due to parsing error: %v\n", err)
			continue
		}

		if start == nil || end == nil {
			startDate := entry.timestamp.Format("2006-01-02")
			startParsed, _ := time.Parse("2006-01-02 15:04:05", startDate+" "+*startTime)
			start = &startParsed

			endDate := entry.timestamp.Format("2006-01-02")
			endParsed, _ := time.Parse("2006-01-02 15:04:05", endDate+" "+*endTime)
			end = &endParsed
		}

		if entry.timestamp.Before(*start) || entry.timestamp.After(*end) {
			continue
		}

		// If the batch is empty, set the interval start to the current entry's timestamp
		if len(batch) == 0 {
			lastMinute = entry.timestamp.Minute()
			intervalStart = entry.timestamp
		}

		// Determine the end of the current interval based on the speed multiplier
		thisIntervalDuration := time.Duration(speedMultiplier) * time.Second
		intervalEnd := intervalStart.Add(thisIntervalDuration)

		// Check if the entry belongs in the current batch
		if entry.timestamp.Before(intervalEnd) || entry.timestamp.Equal(intervalEnd) {
			batch = append(batch, entry)
		} else {
			if entry.timestamp.Minute() != lastMinute {
				fmt.Fprintf(os.Stderr, " %s Logtime: %s\n", time.Now().Format("15:04:05"), entry.timestamp)
				lastMinute = entry.timestamp.Minute()
			}
			// Replay the current batch and reset for the next interval
			replayBatch(batch)
			batch = []LogEntry{entry} // Start new batch with the current entry
			intervalStart = entry.timestamp

			// Sleep to simulate the interval (e.g., 1 second real time)
			// replay batch just slept for 3x150 so now sleep for 450
			time.Sleep(450 * time.Millisecond)
		}
	}

	// Replay any remaining log lines in the last batch
	if len(batch) > 0 {
		replayBatch(batch)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading log file: %w", err)
	}

	return nil
}

const numChunks = 3 // Divide the batch into thirds
const fixedDelay = 150

func replayBatch(batch []LogEntry) {
	if len(batch) == 0 {
		return
	}

	// Calculate chunk size and fixedDelay
	chunkSize := (len(batch) + numChunks - 1) / numChunks // Round up

	for i := 0; i < len(batch); i += chunkSize {
		// Process a chunk of entries
		end := i + chunkSize
		if end > len(batch) {
			end = len(batch)
		}

		for _, entry := range batch[i:end] {
			fmt.Println(entry.line)
		}

		// Sleep after each chunk, except the last one
		if end <= len(batch) {
			time.Sleep(fixedDelay)
		}
	}
}

// replayBatch outputs all lines in the given batch at once
func replayBatchOld(batch []LogEntry) {
	for _, entry := range batch {
		fmt.Println(entry.line)
	}
}

var startTime, endTime *string

func main() {
	filename := flag.String("file", "./access.log", "Path to the NGINX log file")
	speedMultiplier := flag.Float64("speed", 1.0, "Speed multiplier for replaying logs (e.g., 2.0 for 2x)")

	startTime = flag.String("start", "00:00:00", "Start time for replay (e.g., '08:00:00')")
	endTime = flag.String("end", "23:59:59", "End time for replay (e.g., '09:30:00')")

	// Custom help message
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Replays log entries from an NGINX standard access log file with configurable speed and time filtering.\n")
		fmt.Fprintf(os.Stderr, "  NOTE: Time bounding only applies to the first day of the log.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s -file ./access.log -speed 2 -start 08:00:00 -end 09:30:00\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -file /var/log/nginx/access.log\n", os.Args[0])
	}

	flag.Parse()

	// Check for extra arguments
	if len(flag.Args()) > 0 {
		fmt.Fprintf(os.Stderr, "Error: unexpected arguments: %v\n", flag.Args())
		os.Exit(1)
	}

	if *filename == "" {
		fmt.Fprintln(os.Stderr, "Error: Please provide a log file path using -file flag")
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Replaying log entries from %s at %.1fx speed...\n", *filename, *speedMultiplier)
	if err := replayLogStream(*filename, *speedMultiplier); err != nil {
		fmt.Fprintf(os.Stderr, "Error during replay: %v\n", err)
		os.Exit(1)
	}
}
