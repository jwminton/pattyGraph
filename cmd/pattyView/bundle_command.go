package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pattyGraph/cmd/pattyView/internal/investigation"
)

type bundleOptions struct {
	inputPath      string
	outputPath     string
	sessionID      string
	fromLogTime    time.Time
	throughLogTime time.Time
}

func parseBundleOptions(inputPath, fromText, throughText, sessionID, outputPath string) (bundleOptions, error) {
	if strings.TrimSpace(inputPath) == "" {
		return bundleOptions{}, errors.New("--bundle requires a PattyLog JSONL file")
	}
	if strings.TrimSpace(fromText) == "" {
		return bundleOptions{}, errors.New("--bundle requires --from")
	}
	if strings.TrimSpace(throughText) == "" {
		return bundleOptions{}, errors.New("--bundle requires --through")
	}
	fromLogTime, err := time.Parse(time.RFC3339Nano, fromText)
	if err != nil {
		return bundleOptions{}, fmt.Errorf("invalid --from log-time %q: %w", fromText, err)
	}
	throughLogTime, err := time.Parse(time.RFC3339Nano, throughText)
	if err != nil {
		return bundleOptions{}, fmt.Errorf("invalid --through log-time %q: %w", throughText, err)
	}
	if fromLogTime.After(throughLogTime) {
		return bundleOptions{}, errors.New("--from log-time is after --through log-time")
	}
	if outputPath == "" {
		outputPath = defaultBundleOutputPath(inputPath, fromLogTime, throughLogTime)
	} else if !strings.HasSuffix(strings.ToLower(outputPath), ".zip") {
		outputPath += ".zip"
	}
	return bundleOptions{
		inputPath:      inputPath,
		outputPath:     outputPath,
		sessionID:      sessionID,
		fromLogTime:    fromLogTime,
		throughLogTime: throughLogTime,
	}, nil
}

func createIncidentBundle(options bundleOptions, stdout io.Writer) error {
	input, err := os.Open(options.inputPath)
	if err != nil {
		return fmt.Errorf("open PattyLog %s: %w", options.inputPath, err)
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return fmt.Errorf("inspect PattyLog %s: %w", options.inputPath, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("PattyLog %s is not a regular file", options.inputPath)
	}

	plan, err := investigation.PlanSelection(input, investigation.SelectionRequest{
		SessionID:      options.sessionID,
		FromLogTime:    options.fromLogTime,
		ThroughLogTime: options.throughLogTime,
		SourceName:     options.inputPath,
		CreatorVersion: PattyViewVersion,
	})
	if err != nil {
		return fmt.Errorf("select incident range: %w", err)
	}
	if err := writeBundleFile(options.outputPath, input, plan); err != nil {
		return err
	}

	manifest := plan.Manifest
	fmt.Fprintf(stdout, "Created incident bundle %s: session %s, intervals %d-%d (%d intervals, %d records)\n",
		options.outputPath,
		manifest.PattyLog.SessionID,
		manifest.Range.FromInterval,
		manifest.Range.ThroughInterval,
		manifest.Range.IntervalCount,
		manifest.PattyLog.RecordCount,
	)
	return nil
}

func writeBundleFile(path string, input io.ReadSeeker, plan *investigation.SelectionPlan) (err error) {
	output, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("incident bundle already exists: %s", path)
		}
		return fmt.Errorf("create incident bundle %s: %w", path, err)
	}
	complete := false
	defer func() {
		if !complete {
			_ = output.Close()
			_ = os.Remove(path)
		}
	}()

	if err := investigation.WriteBundle(output, input, plan); err != nil {
		return fmt.Errorf("write incident bundle %s: %w", path, err)
	}
	if err := output.Sync(); err != nil {
		return fmt.Errorf("sync incident bundle %s: %w", path, err)
	}
	if err := output.Close(); err != nil {
		return fmt.Errorf("close incident bundle %s: %w", path, err)
	}
	complete = true
	return nil
}

func defaultBundleOutputPath(inputPath string, fromLogTime, throughLogTime time.Time) string {
	base := filepath.Base(inputPath)
	stem := compactIncidentStem(base)
	name := fmt.Sprintf("%s_%s.incident.zip", stem, compactLogTimeRange(fromLogTime, throughLogTime))
	return filepath.Join(filepath.Dir(inputPath), name)
}

func compactIncidentStem(base string) string {
	stem := strings.TrimSpace(base)
	for {
		matched := false
		lower := strings.ToLower(stem)
		for _, suffix := range []string{".jsonl", ".zip", ".incident"} {
			if strings.HasSuffix(lower, suffix) {
				stem = stem[:len(stem)-len(suffix)]
				matched = true
				break
			}
		}
		if !matched {
			break
		}
	}
	runes := []rune(stem)
	if len(runes) > 32 {
		stem = string(runes[:32])
	}
	if stem == "" {
		return "pattyLog"
	}
	return stem
}

func compactLogTimeRange(fromLogTime, throughLogTime time.Time) string {
	fromDate := fromLogTime.Format("20060102")
	throughDate := throughLogTime.Format("20060102")
	if fromDate == throughDate {
		return fmt.Sprintf("%s_%s-%s", fromDate, fromLogTime.Format("1504"), throughLogTime.Format("1504"))
	}
	return fmt.Sprintf("%s_%s-%s_%s", fromDate, fromLogTime.Format("1504"), throughDate, throughLogTime.Format("1504"))
}
