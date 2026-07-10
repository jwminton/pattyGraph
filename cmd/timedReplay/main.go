// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

const nginxTimeLayout = "02/Jan/2006:15:04:05 -0700"

type replayConfig struct {
	sourcePath string
	outputPath string
	speed      float64
	sync0      bool
}

type logGroup struct {
	second time.Time
	lines  []string
}

type progressTracker struct {
	groupsWritten int
	linesWritten  int
	lastPrinted   time.Time
}

func main() {
	conf := parseFlags()
	if err := run(conf); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func parseFlags() replayConfig {
	conf := replayConfig{}
	flag.StringVar(&conf.sourcePath, "file", "./access.log", "Path to the source NGINX access log")
	flag.StringVar(&conf.outputPath, "out", "-", "Output path to append replayed lines to, or '-' for stdout")
	flag.Float64Var(&conf.speed, "speed", 1.0, "Replay speed multiplier, e.g. 2.0 replays two log seconds per real second")
	flag.BoolVar(&conf.sync0, "sync0", false, "Wait before first output until wall-clock seconds match the first valid log line")
	flag.Parse()
	return conf
}

func run(conf replayConfig) error {
	if conf.speed <= 0 {
		return fmt.Errorf("speed must be greater than 0")
	}

	source, err := os.Open(conf.sourcePath)
	if err != nil {
		return fmt.Errorf("open source log %q: %w", conf.sourcePath, err)
	}
	defer source.Close()
	sourceInfo, err := source.Stat()
	if err != nil {
		return fmt.Errorf("stat source log %q: %w", conf.sourcePath, err)
	}
	if err := rejectReplayOutputOverlap(conf.sourcePath, conf.outputPath, sourceInfo); err != nil {
		return err
	}

	out, closeOut, err := openOutput(conf.outputPath)
	if err != nil {
		return err
	}
	if closeOut != nil {
		defer closeOut()
	}

	fmt.Fprintf(os.Stderr, "timedReplay: replaying %s -> %s at %.2fx\n", conf.sourcePath, conf.outputPath, conf.speed)
	delay := time.Duration(float64(time.Second) / conf.speed)

	return replay(source, out, delay, conf.sync0)
}

func rejectReplayOutputOverlap(sourcePath, outputPath string, sourceInfo os.FileInfo) error {
	if outputPath == "-" {
		outInfo, err := os.Stdout.Stat()
		if err != nil || !outInfo.Mode().IsRegular() {
			return nil
		}
		if os.SameFile(sourceInfo, outInfo) {
			return fmt.Errorf("stdout is redirected to the source log %q; replay output must use a different file", sourcePath)
		}
		return nil
	}

	outInfo, err := os.Stat(outputPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat output log %q: %w", outputPath, err)
	}
	if os.SameFile(sourceInfo, outInfo) {
		return fmt.Errorf("output log %q is the same file as source log %q; replay output must use a different file", outputPath, sourcePath)
	}
	return nil
}

func openOutput(path string) (io.Writer, func() error, error) {
	if path == "-" {
		return os.Stdout, nil, nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open output log %q: %w", path, err)
	}
	return f, f.Close, nil
}

func replay(source io.Reader, out io.Writer, delay time.Duration, sync0 bool) error {
	scanner := bufio.NewScanner(source)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	writer := bufio.NewWriter(out)
	defer writer.Flush()

	var current logGroup
	haveGroup := false
	lineNumber := 0
	progress := progressTracker{}
	didStart := false

	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		logSecond, err := parseNginxLogTime(line)
		if err != nil {
			warnDropLine(lineNumber, err)
			continue
		}

		if !haveGroup {
			current = logGroup{second: logSecond, lines: []string{line}}
			haveGroup = true
			continue
		}

		if logSecond.Equal(current.second) {
			current.lines = append(current.lines, line)
			continue
		}

		maybeStartReplay(sync0, &didStart, current)
		if err := writeGroup(writer, current); err != nil {
			return err
		}
		progress.record(current)
		time.Sleep(delay)

		current = logGroup{second: logSecond, lines: []string{line}}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read source log: %w", err)
	}

	if haveGroup {
		maybeStartReplay(sync0, &didStart, current)
		if err := writeGroup(writer, current); err != nil {
			return err
		}
		progress.record(current)
	}

	fmt.Fprintf(os.Stderr, "timedReplay: complete groups=%d lines=%d\n", progress.groupsWritten, progress.linesWritten)
	return nil
}

func maybeStartReplay(sync0 bool, didStart *bool, firstGroup logGroup) {
	if *didStart {
		return
	}
	if sync0 {
		waitForMatchingSecond(firstGroup.second.Second())
	}
	*didStart = true
	fmt.Fprintf(os.Stderr, "timedReplay: started replaying at log second %s\n", firstGroup.second.Format(time.RFC3339))
}

func (p *progressTracker) record(group logGroup) {
	p.groupsWritten++
	p.linesWritten += len(group.lines)
	if group.second.Second() != 0 {
		return
	}
	if !p.lastPrinted.IsZero() && group.second.Sub(p.lastPrinted) < time.Minute {
		return
	}
	p.lastPrinted = group.second
	fmt.Fprintf(os.Stderr, "timedReplay: %s groups=%d lines=%d\n", group.second.Format(time.RFC3339), p.groupsWritten, p.linesWritten)
}

func waitForMatchingSecond(targetSecond int) {
	if targetSecond < 0 || targetSecond > 59 {
		return
	}
	now := time.Now()
	waitSeconds := (targetSecond - now.Second() + 60) % 60
	if waitSeconds == 0 {
		fmt.Fprintf(os.Stderr, "timedReplay: sync0 matched wall second %02d\n", targetSecond)
		return
	}
	fmt.Fprintf(os.Stderr, "timedReplay: sync0 waiting %ds for wall second %02d\n", waitSeconds, targetSecond)
	time.Sleep(time.Duration(waitSeconds) * time.Second)
}

func parseNginxLogTime(line string) (time.Time, error) {
	start := strings.IndexByte(line, '[')
	if start == -1 {
		return time.Time{}, fmt.Errorf("timestamp start not found")
	}
	endRel := strings.IndexByte(line[start+1:], ']')
	if endRel == -1 {
		return time.Time{}, fmt.Errorf("timestamp end not found")
	}
	raw := line[start+1 : start+1+endRel]
	t, err := time.Parse(nginxTimeLayout, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse timestamp %q: %w", raw, err)
	}
	return t.Truncate(time.Second), nil
}

func writeGroup(writer *bufio.Writer, group logGroup) error {
	for _, line := range group.lines {
		if _, err := writer.WriteString(line); err != nil {
			return fmt.Errorf("write replay line: %w", err)
		}
		if err := writer.WriteByte('\n'); err != nil {
			return fmt.Errorf("write replay newline: %w", err)
		}
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush replay output: %w", err)
	}
	return nil
}

func warnDropLine(lineNumber int, err error) {
	fmt.Fprintf(os.Stderr, "warn: dropping unparsable line %d: %v\n", lineNumber, err)
}
