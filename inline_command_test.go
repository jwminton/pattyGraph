// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import "testing"

func matcherIndexByNameForTest(name string) int {
	for i, matcher := range PattyGraph.matchers {
		if matcher.matcherName() == name {
			return i
		}
	}
	return -1
}

func TestInlineAddUndecoratedNameInsertsBeforeBots(t *testing.T) {
	setupMonitorPipelineTestGraph()

	invokeInlineCommand("!!! add googlebot")

	googlebotIndex := matcherIndexByNameForTest("googlebot")
	if googlebotIndex == -1 {
		t.Fatal("googlebot matcher was not added")
	}
	if googlebotIndex != botsIndex-1 {
		t.Fatalf("googlebot index = %d, want immediately before Bots at %d", googlebotIndex, botsIndex-1)
	}
}

func TestInlineAddPlusNameInsertsFirst(t *testing.T) {
	setupMonitorPipelineTestGraph()

	invokeInlineCommand("!!! add +googlebot")

	if got := PattyGraph.matchers[0].matcherName(); got != "googlebot" {
		t.Fatalf("first matcher = %q, want googlebot", got)
	}
}

func TestInlineAddMinusNameInsertsBeforeBots(t *testing.T) {
	setupMonitorPipelineTestGraph()

	invokeInlineCommand("!!! add -googlebot")

	googlebotIndex := matcherIndexByNameForTest("googlebot")
	if googlebotIndex == -1 {
		t.Fatal("googlebot matcher was not added")
	}
	if googlebotIndex != botsIndex-1 {
		t.Fatalf("googlebot index = %d, want immediately before Bots at %d", googlebotIndex, botsIndex-1)
	}
}

func TestInlineAddStarNameInsertsBelowBotsBeforeLines(t *testing.T) {
	setupMonitorPipelineTestGraph()

	invokeInlineCommand("!!! add *googlebot")

	googlebotIndex := matcherIndexByNameForTest("googlebot")
	linesIndex := matcherIndexByNameForTest("lines")
	if googlebotIndex == -1 {
		t.Fatal("googlebot matcher was not added")
	}
	if googlebotIndex != botsIndex+1 {
		t.Fatalf("googlebot index = %d, want immediately after Bots at %d", googlebotIndex, botsIndex+1)
	}
	if googlebotIndex != linesIndex-1 {
		t.Fatalf("googlebot index = %d, want immediately before lines at %d", googlebotIndex, linesIndex-1)
	}
}

func TestInlineDelUndecoratedNameRemovesMatcher(t *testing.T) {
	setupMonitorPipelineTestGraph()
	invokeInlineCommand("!!! add googlebot")

	invokeInlineCommand("!!! del googlebot")

	if matcherIndexByNameForTest("googlebot") != -1 {
		t.Fatal("googlebot matcher still exists after undecorated del")
	}
}

func TestInlineDelDecoratedNameRemovesMatcher(t *testing.T) {
	setupMonitorPipelineTestGraph()
	invokeInlineCommand("!!! add googlebot")

	invokeInlineCommand("!!! del *googlebot")

	if matcherIndexByNameForTest("googlebot") != -1 {
		t.Fatal("googlebot matcher still exists after decorated del")
	}
}

func TestInlineControlCommandTogglesControlFileProcessing(t *testing.T) {
	setupMonitorPipelineTestGraph()
	enableControlFile = false

	invokeInlineCommand("!!! control")
	if !enableControlFile {
		t.Fatal("enableControlFile = false after !!! control, want true")
	}

	invokeInlineCommand("!!! control off")
	if enableControlFile {
		t.Fatal("enableControlFile = true after !!! control off, want false")
	}

	invokeInlineCommand("!!! control on")
	if !enableControlFile {
		t.Fatal("enableControlFile = false after !!! control on, want true")
	}
}
