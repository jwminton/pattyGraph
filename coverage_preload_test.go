// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func coverageNginxLine(minute, second int, path string) string {
	return fmt.Sprintf(`192.0.2.%d - - [13/May/2026:14:%02d:%02d -0700] "GET %s HTTP/1.1" 200 321 "-" "CoverageAgent/1.0"`, minute+1, minute, second, path)
}

func TestCoveragePreloadRecentMinutes(t *testing.T) {
	setupMonitorPipelineTestGraph()
	forceZeroStart = true
	PattyGraph.botsMatcher.disableAutoAdd = true
	PattyGraph.pattyConfig.mbToRead = 1
	lines := []string{
		"unparseable",
		coverageNginxLine(20, 1, "/ignored-first-minute"),
		coverageNginxLine(21, 2, "/catalog"),
		coverageNginxLine(21, 3, "/catalog?page=2"),
		coverageNginxLine(22, 4, "/checkout"),
	}
	path := filepath.Join(t.TempDir(), "access.log")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	PattyGraph.filePath = path
	if err := preloadRecentMinutes(); err != nil {
		t.Fatal(err)
	}
	if PattyGraph.intervalsCompleted != 2 || logicalCycles != 120 || PattyGraph.totalLines != 3 {
		t.Fatalf("preload state intervals=%d cycles=%d lines=%d", PattyGraph.intervalsCompleted, logicalCycles, PattyGraph.totalLines)
	}

	PattyGraph.pattyConfig.mbToRead = 0
	if err := preloadRecentMinutes(); err != nil {
		t.Fatal(err)
	}
	PattyGraph.pattyConfig.mbToRead = 1
	PattyGraph.filePath = filepath.Join(t.TempDir(), "missing.log")
	if err := preloadRecentMinutes(); err == nil {
		t.Fatal("missing preload source was accepted")
	}
}
