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

func TestInlineAddScopedIPReturnsResult(t *testing.T) {
	setupMonitorPipelineTestGraph()

	result := invokeInlineCommand("!!! add ip-91 --ips 91.99.72.15")

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q", result.Status, InlineCommandStatusApplied)
	}
	if result.Result["action"] != "add_matcher" {
		t.Fatalf("action = %v, want add_matcher", result.Result["action"])
	}
	if result.Result["matcher_name"] != "ip-91" {
		t.Fatalf("matcher_name = %v, want ip-91", result.Result["matcher_name"])
	}
	if result.Result["scope"] != "ips" {
		t.Fatalf("scope = %v, want ips", result.Result["scope"])
	}
	patterns, ok := result.Result["patterns"].([]string)
	if !ok || len(patterns) != 1 || patterns[0] != "91.99.72.15" {
		t.Fatalf("patterns = %#v, want [91.99.72.15]", result.Result["patterns"])
	}
}

func TestInlineBadAddReturnsRejectedResult(t *testing.T) {
	setupMonitorPipelineTestGraph()

	result := invokeInlineCommand("!!! add")

	if result.Status != InlineCommandStatusRejected {
		t.Fatalf("status = %q, want %q", result.Status, InlineCommandStatusRejected)
	}
	if result.Result["action"] != "add_matcher" {
		t.Fatalf("action = %v, want add_matcher", result.Result["action"])
	}
	if result.Result["error"] == "" {
		t.Fatal("error was empty")
	}
}
