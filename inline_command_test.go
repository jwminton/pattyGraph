// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func matcherIndexByNameForTest(name string) int {
	for i, matcher := range PattyGraph.matchers {
		if matcher.matcherName() == name {
			return i
		}
	}
	return -1
}

func matcherCountByNameForTest(name string) int {
	count := 0
	for _, matcher := range PattyGraph.matchers {
		if matcher.matcherName() == name {
			count++
		}
	}
	return count
}

func assertInlineInvalidArgument(t *testing.T, result InlineCommandResult, argument, value, message string) {
	t.Helper()
	if result.Status != InlineCommandStatusRejected {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusRejected, result.Result)
	}
	if result.Result["error_kind"] != "invalid_argument" {
		t.Fatalf("error_kind = %v, want invalid_argument", result.Result["error_kind"])
	}
	if result.Result["argument"] != argument {
		t.Fatalf("argument = %v, want %s", result.Result["argument"], argument)
	}
	if result.Result["value"] != value {
		t.Fatalf("value = %v, want %q", result.Result["value"], value)
	}
	if result.Result["error"] != message {
		t.Fatalf("error = %v, want %q", result.Result["error"], message)
	}
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

func TestInlineAddRejectsDuplicateMatcherName(t *testing.T) {
	setupMonitorPipelineTestGraph()

	first := invokeInlineCommand("!!! add googlebot")
	if first.Status != InlineCommandStatusApplied {
		t.Fatalf("first status = %q, want applied: %#v", first.Status, first.Result)
	}

	duplicate := invokeInlineCommand("!!! add googlebot")
	if duplicate.Status != InlineCommandStatusRejected {
		t.Fatalf("duplicate status = %q, want rejected: %#v", duplicate.Status, duplicate.Result)
	}
	if duplicate.Result["error"] != "duplicate matcher name" {
		t.Fatalf("duplicate error = %v, want duplicate matcher name", duplicate.Result["error"])
	}
	if got := matcherCountByNameForTest("googlebot"); got != 1 {
		t.Fatalf("googlebot matcher count = %d, want 1", got)
	}
}

func TestInlineAddRejectsMatcherNameContainingWhitespace(t *testing.T) {
	setupMonitorPipelineTestGraph()

	for _, command := range []string{
		"!!! add 'space probe' image",
		"!!! add --words 'space probe' image",
		"!!! add +'space probe' image",
	} {
		result := invokeInlineCommand(command)
		if result.Status != InlineCommandStatusRejected {
			t.Fatalf("%s status = %q, want %q: %#v", command, result.Status, InlineCommandStatusRejected, result.Result)
		}
		if result.Result["error"] != "matcher name cannot contain whitespace" {
			t.Fatalf("%s error = %v, want matcher name cannot contain whitespace", command, result.Result["error"])
		}
		if result.Result["error_kind"] != "invalid_argument" || result.Result["argument"] != "matcher_name" {
			t.Fatalf("%s invalid argument fields = %#v", command, result.Result)
		}
		if result.Result["raw_matcher_name"] == "" {
			t.Fatalf("%s raw_matcher_name was empty", command)
		}
		if result.Result["normalized_matcher_name"] != "space probe" {
			t.Fatalf("%s normalized_matcher_name = %v, want space probe", command, result.Result["normalized_matcher_name"])
		}
	}

	if matcherNameExists("space probe") {
		t.Fatal("matcher with whitespace name was added")
	}
}

func TestInlineAddMultiPatternLineMatcherUsesORPatterns(t *testing.T) {
	setupMonitorPipelineTestGraph()

	result := invokeInlineCommand("!!! add bad-paths .css wslogin xmlrpc")

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want applied: %#v", result.Status, result.Result)
	}
	if result.Result["matcher_name"] != "bad-paths" {
		t.Fatalf("matcher_name = %v, want bad-paths", result.Result["matcher_name"])
	}
	if result.Result["scope"] != "line" {
		t.Fatalf("scope = %v, want line", result.Result["scope"])
	}
	if result.Result["placement"] != "before_bots" {
		t.Fatalf("placement = %v, want before_bots", result.Result["placement"])
	}
	patterns, ok := result.Result["patterns"].([]string)
	if !ok || strings.Join(patterns, "|") != ".css|wslogin|xmlrpc" {
		t.Fatalf("patterns = %#v, want [.css wslogin xmlrpc]", result.Result["patterns"])
	}

	matcherIndex := matcherIndexByNameForTest("bad-paths")
	if matcherIndex == -1 {
		t.Fatal("bad-paths matcher was not added")
	}
	if matcherIndex != botsIndex-1 {
		t.Fatalf("bad-paths index = %d, want immediately before Bots at %d", matcherIndex, botsIndex-1)
	}
	badPaths := PattyGraph.matchers[matcherIndex].asMatcher()

	match(standardPipelineLine("192.0.2.10", "/assets/site.css", "200", "100", "-", "Mozilla/5.0"))
	if badPaths.intervalCount != 1 {
		t.Fatalf(".css line count = %d, want 1", badPaths.intervalCount)
	}

	match(standardPipelineLine("192.0.2.11", "/wp-login.php", "404", "100", "-", "wslogin probe"))
	if badPaths.intervalCount != 2 {
		t.Fatalf("wslogin line count = %d, want 2", badPaths.intervalCount)
	}

	match(standardPipelineLine("192.0.2.12", "/xmlrpc.php", "404", "100", "-", "Mozilla/5.0"))
	if badPaths.intervalCount != 3 {
		t.Fatalf("xmlrpc line count = %d, want 3", badPaths.intervalCount)
	}

	match(standardPipelineLine("192.0.2.13", "/ordinary", "200", "100", "-", "Mozilla/5.0"))
	if badPaths.intervalCount != 3 {
		t.Fatalf("ordinary line count = %d, want unchanged 3", badPaths.intervalCount)
	}
}

func TestInlineJSONFileImpliesJSONOutput(t *testing.T) {
	setupMonitorPipelineTestGraph()
	generateSidecarJSONL = false
	PattyGraph.pattyConfig.setSaveDir("/tmp/patty")

	result := invokeInlineCommand("!!! json-file current.jsonl")

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want applied: %#v", result.Status, result.Result)
	}
	if !generateSidecarJSONL {
		t.Fatal("generateSidecarJSONL = false, want true")
	}
	if got, want := PattyGraph.pattyConfig.jsonFile, "/tmp/patty/current.jsonl"; got != want {
		t.Fatalf("jsonFile = %q, want %q", got, want)
	}
}

func TestInlineJSONTogglesSidecarOutput(t *testing.T) {
	setupMonitorPipelineTestGraph()
	generateSidecarJSONL = false

	invokeInlineCommand("!!! json on")
	if !generateSidecarJSONL {
		t.Fatal("json on did not enable generateSidecarJSONL")
	}

	invokeInlineCommand("!!! json off")
	if generateSidecarJSONL {
		t.Fatal("json off did not disable generateSidecarJSONL")
	}

	invokeInlineCommand("!!! json-file current.jsonl")
	invokeInlineCommand("!!! json off")
	if generateSidecarJSONL {
		t.Fatal("json off did not disable output with json-file configured")
	}
}

func TestInlineJSONRejectsInvalidBooleanValue(t *testing.T) {
	setupMonitorPipelineTestGraph()
	generateSidecarJSONL = false

	result := invokeInlineCommand("!!! json maybe")

	assertInlineInvalidArgument(t, result, "json", "maybe", "json requires a boolean value")
	if generateSidecarJSONL {
		t.Fatal("json maybe enabled JSONL output")
	}
}

func TestInlineJSONSourcesIsIndependentFromJSONOutput(t *testing.T) {
	setupMonitorPipelineTestGraph()

	result := invokeInlineCommand("!!! json-sources on")
	if result.Status != InlineCommandStatusApplied || result.Result["enabled"] != true {
		t.Fatalf("json-sources on result = %#v", result)
	}
	if !includeSidecarSourceExamples {
		t.Fatal("json-sources on did not enable source examples")
	}
	if generateSidecarJSONL {
		t.Fatal("json-sources on enabled JSONL output")
	}

	invokeInlineCommand("!!! json on")
	invokeInlineCommand("!!! json off")
	if !includeSidecarSourceExamples {
		t.Fatal("json output toggles reset source examples")
	}

	result = invokeInlineCommand("!!! json-sources off")
	if result.Status != InlineCommandStatusApplied || result.Result["enabled"] != false {
		t.Fatalf("json-sources off result = %#v", result)
	}
	if includeSidecarSourceExamples || generateSidecarJSONL {
		t.Fatal("json-sources off changed the wrong output state")
	}
}

func TestInlineJSONSourcesRejectsInvalidBooleanValue(t *testing.T) {
	setupMonitorPipelineTestGraph()

	result := invokeInlineCommand("!!! json-sources maybe")

	assertInlineInvalidArgument(t, result, "json-sources", "maybe", "json-sources requires a boolean value")
	if includeSidecarSourceExamples || generateSidecarJSONL {
		t.Fatal("invalid json-sources value changed output state")
	}
}

func TestInlineAddRejectsDuplicateMatcherNameAcrossPlacementPrefixes(t *testing.T) {
	setupMonitorPipelineTestGraph()

	first := invokeInlineCommand("!!! add +googlebot")
	if first.Status != InlineCommandStatusApplied {
		t.Fatalf("first status = %q, want applied: %#v", first.Status, first.Result)
	}

	duplicate := invokeInlineCommand("!!! add *googlebot")
	if duplicate.Status != InlineCommandStatusRejected {
		t.Fatalf("duplicate status = %q, want rejected: %#v", duplicate.Status, duplicate.Result)
	}
	if got := matcherCountByNameForTest("googlebot"); got != 1 {
		t.Fatalf("googlebot matcher count = %d, want 1", got)
	}
}

func TestInlineAddMatcherNamesAreCaseSensitive(t *testing.T) {
	setupMonitorPipelineTestGraph()

	lower := invokeInlineCommand("!!! add googlebot")
	if lower.Status != InlineCommandStatusApplied {
		t.Fatalf("lower status = %q, want applied: %#v", lower.Status, lower.Result)
	}
	upper := invokeInlineCommand("!!! add Googlebot")
	if upper.Status != InlineCommandStatusApplied {
		t.Fatalf("upper status = %q, want applied: %#v", upper.Status, upper.Result)
	}

	if got := matcherCountByNameForTest("googlebot"); got != 1 {
		t.Fatalf("googlebot matcher count = %d, want 1", got)
	}
	if got := matcherCountByNameForTest("Googlebot"); got != 1 {
		t.Fatalf("Googlebot matcher count = %d, want 1", got)
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

func TestInlineDelRejectsMissingMatcher(t *testing.T) {
	setupMonitorPipelineTestGraph()

	result := invokeInlineCommand("!!! del noSuchMatcher")

	if result.Status != InlineCommandStatusRejected {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusRejected, result.Result)
	}
	if result.Result["error"] != "matcher not found" {
		t.Fatalf("error = %v, want matcher not found", result.Result["error"])
	}
	if result.Result["matcher_name"] != "noSuchMatcher" {
		t.Fatalf("matcher_name = %v, want noSuchMatcher", result.Result["matcher_name"])
	}
}

func TestInlineDelRejectsProtectedSystemMatcher(t *testing.T) {
	setupMonitorPipelineTestGraph()

	result := invokeInlineCommand("!!! del lines")

	if result.Status != InlineCommandStatusRejected {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusRejected, result.Result)
	}
	if result.Result["error"] != "matcher cannot be deleted" {
		t.Fatalf("error = %v, want matcher cannot be deleted", result.Result["error"])
	}
	if matcherIndexByNameForTest("lines") == -1 {
		t.Fatal("lines matcher was removed")
	}
}

func TestInlineAddBotsEnablesAutoAdd(t *testing.T) {
	setupMonitorPipelineTestGraph()
	PattyGraph.botsMatcher.disableAutoAdd = true

	result := invokeInlineCommand("!!! add Bots")

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusApplied, result.Result)
	}
	if result.Result["action"] != "enable_bots_auto_add" {
		t.Fatalf("action = %v, want enable_bots_auto_add", result.Result["action"])
	}
	if matcherIndexByNameForTest(BotsMatcherName) == -1 {
		t.Fatal("Bots matcher was removed")
	}
	if matcherCountByNameForTest(BotsMatcherName) != 1 {
		t.Fatalf("Bots matcher count = %d, want 1", matcherCountByNameForTest(BotsMatcherName))
	}
	if PattyGraph.botsMatcher.disableAutoAdd {
		t.Fatal("Bots disableAutoAdd = true, want false")
	}
}

func TestInlineDelBotsDisablesAutoAdd(t *testing.T) {
	setupMonitorPipelineTestGraph()
	PattyGraph.botsMatcher.disableAutoAdd = false

	result := invokeInlineCommand("!!! del Bots")

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusApplied, result.Result)
	}
	if result.Result["action"] != "disable_bots_auto_add" {
		t.Fatalf("action = %v, want disable_bots_auto_add", result.Result["action"])
	}
	if matcherIndexByNameForTest(BotsMatcherName) == -1 {
		t.Fatal("Bots matcher was removed")
	}
	if !PattyGraph.botsMatcher.disableAutoAdd {
		t.Fatal("Bots disableAutoAdd = false, want true")
	}
}

func TestInlineAddOptionalBuiltinsDefaultBelowBots(t *testing.T) {
	tests := []struct {
		name       string
		command    string
		builtin    string
		pattern    string
		matchingUA string
	}{
		{
			name:       "Browser flag first",
			command:    "!!! add --builtin browser # stock browser",
			builtin:    BrowserMatcherName,
			pattern:    browserRegexString,
			matchingUA: "Mozilla/5.0 Chrome/126.0",
		},
		{
			name:       "Platform flag second",
			command:    "!!! add Platform --builtin # stock platform",
			builtin:    PlatformMatcherName,
			pattern:    platformRegexString,
			matchingUA: "Mozilla/5.0 (FreeBSD) Android",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupMonitorPipelineTestGraph()
			result := invokeInlineCommand(tt.command)
			if result.Status != InlineCommandStatusApplied {
				t.Fatalf("status = %q, want applied: %#v", result.Status, result.Result)
			}
			if result.Result["builtin"] != tt.builtin || result.Result["regex"] != true {
				t.Fatalf("built-in result = %#v", result.Result)
			}
			patterns, ok := result.Result["patterns"].([]string)
			if !ok || len(patterns) != 1 || patterns[0] != tt.pattern {
				t.Fatalf("patterns = %#v, want packaged pattern %q", result.Result["patterns"], tt.pattern)
			}
			index := matcherIndexByNameForTest(tt.builtin)
			if index <= botsIndex || index != matcherIndexByNameForTest("lines")-1 {
				t.Fatalf("%s index = %d, Bots = %d, lines = %d", tt.builtin, index, botsIndex, matcherIndexByNameForTest("lines"))
			}
			matcher := findMatcherByName(tt.builtin)
			*currentLine = lineSource{logLine: tt.matchingUA, ip: "192.0.2.20"}
			if matcher == nil || !matcher.match() {
				t.Fatalf("built-in %s did not match %q", tt.builtin, tt.matchingUA)
			}
			if got := matcher.asInlineCommand(); got != tt.command {
				t.Fatalf("inline command = %q, want retained %q", got, tt.command)
			}
		})
	}
}

func TestInlineAddOptionalBuiltinPlacementModifiers(t *testing.T) {
	tests := []struct {
		name    string
		command string
		check   func(index int) bool
	}{
		{name: "top", command: "!!! add --builtin +Browser", check: func(index int) bool { return index == 0 }},
		{name: "above Bots", command: "!!! add --builtin -Browser", check: func(index int) bool { return index == botsIndex-1 }},
		{name: "beneath Bots", command: "!!! add --builtin *Browser", check: func(index int) bool {
			return index > botsIndex && index == matcherIndexByNameForTest("lines")-1
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupMonitorPipelineTestGraph()
			result := invokeInlineCommand(tt.command)
			if result.Status != InlineCommandStatusApplied {
				t.Fatalf("status = %q, want applied: %#v", result.Status, result.Result)
			}
			if index := matcherIndexByNameForTest(BrowserMatcherName); !tt.check(index) {
				t.Fatalf("Browser index = %d, Bots = %d, lines = %d", index, botsIndex, matcherIndexByNameForTest("lines"))
			}
		})
	}
}

func TestInlinePlainBuiltinNamesCreateLiteralMatchers(t *testing.T) {
	for _, name := range []string{BrowserMatcherName, PlatformMatcherName} {
		t.Run(name, func(t *testing.T) {
			setupMonitorPipelineTestGraph()
			result := invokeInlineCommand("!!! add " + name)
			if result.Status != InlineCommandStatusApplied || result.Result["regex"] != false {
				t.Fatalf("literal add result = %#v", result)
			}
			if _, exists := result.Result["builtin"]; exists {
				t.Fatalf("plain add reported built-in metadata: %#v", result.Result)
			}
			matcher := findMatcherByName(name)
			*currentLine = lineSource{logLine: "Mozilla/5.0 Chrome/126.0 Android", ip: "192.0.2.21"}
			if matcher == nil || matcher.match() {
				t.Fatalf("literal %s unexpectedly matched packaged Browser/Platform content", name)
			}
			currentLine.logLine = "request contains " + name
			if !matcher.match() {
				t.Fatalf("literal %s did not match its own name", name)
			}
		})
	}
}

func TestInlineOptionalBuiltinCanBeReplacedByCustomRegex(t *testing.T) {
	setupMonitorPipelineTestGraph()
	if result := invokeInlineCommand("!!! add --builtin Platform"); result.Status != InlineCommandStatusApplied {
		t.Fatalf("built-in add = %#v", result)
	}
	if result := invokeInlineCommand("!!! del Platform"); result.Status != InlineCommandStatusApplied {
		t.Fatalf("built-in delete = %#v", result)
	}
	result := invokeInlineCommand(`!!! add Platform --regex (FreeBSD|OpenBSD)`)
	if result.Status != InlineCommandStatusApplied || result.Result["regex"] != true {
		t.Fatalf("custom regex add = %#v", result)
	}
	if _, exists := result.Result["builtin"]; exists {
		t.Fatalf("custom matcher reported built-in metadata: %#v", result.Result)
	}
	matcher := findMatcherByName(PlatformMatcherName)
	*currentLine = lineSource{logLine: "Mozilla/5.0 FreeBSD", ip: "192.0.2.22"}
	if matcher == nil || !matcher.match() {
		t.Fatal("custom Platform regex did not match FreeBSD")
	}
}

func TestInlineBuiltinCommentSurvivesConfigGeneration(t *testing.T) {
	setupMonitorPipelineTestGraph()
	const command = "!!! add --builtin Browser #stock broswer here"
	result := invokeInlineCommand(command)
	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want applied: %#v", result.Status, result.Result)
	}
	var config strings.Builder
	if err := writeConfig(&config); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}
	if !strings.Contains(config.String(), command+"\n") {
		t.Fatalf("generated config did not preserve declaration %q:\n%s", command, config.String())
	}
}

func TestInlineBuiltinRejectsCustomPatternsWithGuidance(t *testing.T) {
	for _, command := range []string{
		`!!! add --builtin Browser (Chrome|Lynx)`,
		`!!! add Platform --builtin --regex (FreeBSD|OpenBSD)`,
		`!!! add --builtin Browser --words`,
	} {
		t.Run(command, func(t *testing.T) {
			setupMonitorPipelineTestGraph()
			result := invokeInlineCommand(command)
			if result.Status != InlineCommandStatusRejected {
				t.Fatalf("status = %q, want rejected: %#v", result.Status, result.Result)
			}
			builtin, _ := result.Result["builtin"].(string)
			if builtin != BrowserMatcherName && builtin != PlatformMatcherName {
				t.Fatalf("builtin = %q, want Browser or Platform", builtin)
			}
			wantCommand := InlinePreamble + " add " + builtin + " --regex <pattern>"
			if result.Result["custom_regex_command"] != wantCommand || result.Result["requires_delete_if_present"] != true {
				t.Fatalf("custom guidance = %#v", result.Result)
			}
			if !strings.Contains(fmt.Sprint(result.Result["error"]), "packaged pattern") {
				t.Fatalf("error = %v, want packaged-pattern guidance", result.Result["error"])
			}
		})
	}
}

func TestInlineBuiltinValidationAndBotsSpecialCase(t *testing.T) {
	t.Run("unknown", func(t *testing.T) {
		setupMonitorPipelineTestGraph()
		result := invokeInlineCommand("!!! add --builtin Unknown")
		if result.Status != InlineCommandStatusRejected || result.Result["argument"] != "builtin" {
			t.Fatalf("unknown result = %#v", result)
		}
		if names, ok := result.Result["valid_builtins"].([]string); !ok || len(names) != 3 {
			t.Fatalf("valid_builtins = %#v", result.Result["valid_builtins"])
		}
	})

	t.Run("missing", func(t *testing.T) {
		setupMonitorPipelineTestGraph()
		result := invokeInlineCommand("!!! add --builtin # choose later")
		if result.Status != InlineCommandStatusRejected || result.Result["argument"] != "builtin" {
			t.Fatalf("missing result = %#v", result)
		}
	})

	t.Run("Bots", func(t *testing.T) {
		setupMonitorPipelineTestGraph()
		PattyGraph.botsMatcher.disableAutoAdd = true
		result := invokeInlineCommand("!!! add --builtin Bots")
		if result.Status != InlineCommandStatusApplied || result.Result["action"] != "enable_bots_auto_add" {
			t.Fatalf("Bots result = %#v", result)
		}
		if PattyGraph.botsMatcher.disableAutoAdd || matcherCountByNameForTest(BotsMatcherName) != 1 {
			t.Fatalf("Bots state after add: disabled=%v count=%d", PattyGraph.botsMatcher.disableAutoAdd, matcherCountByNameForTest(BotsMatcherName))
		}
	})

	for _, command := range []string{
		"!!! add --builtin +Bots",
		"!!! add --builtin Bots bot-pattern",
		"!!! add Bots bot-pattern",
		"!!! add Bots --words",
	} {
		t.Run(command, func(t *testing.T) {
			setupMonitorPipelineTestGraph()
			result := invokeInlineCommand(command)
			if result.Status != InlineCommandStatusRejected || result.Result["builtin"] != BotsMatcherName {
				t.Fatalf("Bots rejection = %#v", result)
			}
			if matcherCountByNameForTest(BotsMatcherName) != 1 {
				t.Fatalf("Bots matcher count = %d, want 1", matcherCountByNameForTest(BotsMatcherName))
			}
		})
	}
}

func TestInlineBuiltinNamesAreCaseInsensitiveOnlyDuringAdd(t *testing.T) {
	tests := []struct {
		command       string
		canonicalName string
	}{
		{command: "!!! add --builtin BROWSER", canonicalName: BrowserMatcherName},
		{command: "!!! add pLaTfOrM --builtin", canonicalName: PlatformMatcherName},
		{command: "!!! add --builtin bots", canonicalName: BotsMatcherName},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			setupMonitorPipelineTestGraph()
			if tt.canonicalName == BotsMatcherName {
				PattyGraph.botsMatcher.disableAutoAdd = true
			}
			result := invokeInlineCommand(tt.command)
			if result.Status != InlineCommandStatusApplied {
				t.Fatalf("status = %q, want applied: %#v", result.Status, result.Result)
			}
			if result.Result["matcher_name"] != tt.canonicalName || result.Result["builtin"] != tt.canonicalName {
				t.Fatalf("canonical result = %#v, want %q", result.Result, tt.canonicalName)
			}
			if matcherCountByNameForTest(tt.canonicalName) != 1 {
				t.Fatalf("canonical matcher %q count = %d, want 1", tt.canonicalName, matcherCountByNameForTest(tt.canonicalName))
			}
		})
	}

	setupMonitorPipelineTestGraph()
	if result := invokeInlineCommand("!!! add --builtin browser"); result.Status != InlineCommandStatusApplied {
		t.Fatalf("lowercase built-in add = %#v", result)
	}
	if result := invokeInlineCommand("!!! del browser"); result.Status != InlineCommandStatusRejected {
		t.Fatalf("lowercase matcher delete status = %q, want rejected", result.Status)
	}
	if matcherIndexByNameForTest(BrowserMatcherName) == -1 {
		t.Fatal("case-mismatched delete removed canonical Browser matcher")
	}
	if result := invokeInlineCommand("!!! del Browser"); result.Status != InlineCommandStatusApplied {
		t.Fatalf("canonical matcher delete = %#v", result)
	}
}

func TestBuiltinHelpUsesPackagedPatternsAndCommands(t *testing.T) {
	wordsHelp := printBuiltinWordLists()
	normalizedWordsHelp := strings.ReplaceAll(wordsHelp, "\n     ", "")
	for _, expected := range []string{
		"!!! add --builtin Browser",
		"!!! add --builtin Platform",
		"Regex matching increases startup replay and ongoing runtime cost.",
	} {
		if !strings.Contains(wordsHelp, expected) {
			t.Fatalf("--help words missing %q", expected)
		}
	}
	for _, expected := range []string{browserRegexString, platformRegexString} {
		if !strings.Contains(normalizedWordsHelp, expected) {
			t.Fatalf("--help words missing packaged pattern %q", expected)
		}
	}
	inlineHelp := inlineCommandHelp()
	for _, expected := range []string{
		"!!! add --builtin <Bots|Browser|Platform>",
		"Browser and Platform default beneath",
		"!!! add Platform --regex <custom-pattern>",
		"Regex matchers scan log lines through Go's regex engine",
		"startup replay and ongoing runtime cost",
		"!!! fact print <message>",
		"including '#'",
	} {
		if !strings.Contains(inlineHelp, expected) {
			t.Fatalf("--help inline missing %q", expected)
		}
	}
}

func TestInlineSelectInterestingMatcherByKey(t *testing.T) {
	setupMonitorPipelineTestGraph()
	refs := PattyGraph.refsMatcher
	refs.currentListing = []string{"www.zanbil.ir", "Suspicious", "filter"}
	PattyGraph.selectedInterestingMatcher = nil

	result := invokeInlineCommand("!!! select --refs Suspicious")

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q", result.Status, InlineCommandStatusApplied)
	}
	if result.Result["action"] != "select_interesting" {
		t.Fatalf("action = %v, want select_interesting", result.Result["action"])
	}
	if result.Result["matcher"] != "refs" {
		t.Fatalf("matcher = %v, want refs", result.Result["matcher"])
	}
	if result.Result["selection"] != "Suspicious" {
		t.Fatalf("selection = %v, want Suspicious", result.Result["selection"])
	}
	if result.Result["selection_index"] != 1 {
		t.Fatalf("selection_index = %v, want 1", result.Result["selection_index"])
	}
	if PattyGraph.selectedInterestingMatcher != refs {
		t.Fatal("refs matcher was not selected")
	}
	if refs.selectedKey != "Suspicious" {
		t.Fatalf("selectedKey = %q, want Suspicious", refs.selectedKey)
	}
}

func TestInterestingSelectionPushesCompactFactoid(t *testing.T) {
	setupMonitorPipelineTestGraph()
	facts = NewFactoidGenerator()
	facts.forced = nil
	PattyGraph.showTicker = true
	refs := PattyGraph.refsMatcher
	stats := newWordStats()
	stats.count = 2300
	stats.historyBuf.Push(2400)
	stats.historyBuf.Push(1200)
	stats.source.captureColor = "[green]"
	refs.wordFrequency["checkout"] = stats
	refs.currentListing = []string{"checkout"}

	refs.selectDisplayItem(0)

	msg, _, _ := facts.Next()
	if !strings.Contains(msg, "ref [green]checkout[default]:") {
		t.Fatalf("factoid = %q, want selected ref message", msg)
	}
	if !strings.Contains(msg, "▲2400") || !strings.Contains(msg, "▼1200") || !strings.Contains(msg, "≈1800") {
		t.Fatalf("factoid = %q, want min/max/avg counts", msg)
	}
}

func TestInterestingSelectionFactoidHandlesNoHistory(t *testing.T) {
	setupMonitorPipelineTestGraph()
	facts = NewFactoidGenerator()
	facts.forced = nil
	PattyGraph.showTicker = true
	words := PattyGraph.wordsMatcher
	stats := newWordStats()
	stats.count = 7
	words.wordFrequency["fresh"] = stats
	words.currentListing = []string{"fresh"}

	words.selectDisplayItem(0)

	msg, _, _ := facts.Next()
	if strings.Contains(msg, "NaN") || strings.Contains(msg, "+Inf") || strings.Contains(msg, "-Inf") {
		t.Fatalf("factoid = %q, want finite counts", msg)
	}
	if !strings.Contains(msg, "word [white]fresh[default]: ▲7 ▼7 ≈0") {
		t.Fatalf("factoid = %q, want no-history fallback counts", msg)
	}
}

func TestInterestingSelectionDisplayHandlesNoHistory(t *testing.T) {
	setupMonitorPipelineTestGraph()
	PattyGraph.showTicker = false
	words := PattyGraph.wordsMatcher
	stats := newWordStats()
	stats.count = 7
	words.wordFrequency["fresh"] = stats
	words.currentListing = []string{"fresh"}
	words.selectDisplayItem(0)

	display := words.renderSparklineRow()
	if !strings.Contains(display, "fresh") {
		t.Fatalf("renderSparklineRow() = %q, want selected key", display)
	}
}

func TestInlineSelectWithoutArgsClearsInterestingSelection(t *testing.T) {
	setupMonitorPipelineTestGraph()
	refs := PattyGraph.refsMatcher
	refs.currentListing = []string{"www.zanbil.ir", "Suspicious", "filter"}
	refs.selectDisplayItem(1)
	if PattyGraph.selectedInterestingMatcher != refs {
		t.Fatal("refs matcher was not selected before clear")
	}

	result := invokeInlineCommand("!!! select")

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q", result.Status, InlineCommandStatusApplied)
	}
	if result.Result["action"] != "clear_selection" {
		t.Fatalf("action = %v, want clear_selection", result.Result["action"])
	}
	if PattyGraph.selectedInterestingMatcher != nil {
		t.Fatal("selected interesting matcher was not cleared")
	}
	if refs.selectedKey != "" {
		t.Fatalf("selectedKey = %q, want empty", refs.selectedKey)
	}
}

func TestInlineSelectIsCaseSensitive(t *testing.T) {
	setupMonitorPipelineTestGraph()
	refs := PattyGraph.refsMatcher
	refs.currentListing = []string{"Filter", "filter"}
	PattyGraph.selectedInterestingMatcher = nil

	upper := invokeInlineCommand("!!! select --refs Filter")
	if upper.Status != InlineCommandStatusApplied {
		t.Fatalf("upper status = %q, want %q", upper.Status, InlineCommandStatusApplied)
	}
	if upper.Result["selection"] != "Filter" {
		t.Fatalf("upper selection = %v, want Filter", upper.Result["selection"])
	}
	if refs.selectedKey != "Filter" {
		t.Fatalf("selectedKey = %q, want Filter", refs.selectedKey)
	}

	lower := invokeInlineCommand("!!! select --refs filter")
	if lower.Status != InlineCommandStatusApplied {
		t.Fatalf("lower status = %q, want %q", lower.Status, InlineCommandStatusApplied)
	}
	if lower.Result["selection"] != "filter" {
		t.Fatalf("lower selection = %v, want filter", lower.Result["selection"])
	}
	if refs.selectedKey != "filter" {
		t.Fatalf("selectedKey = %q, want filter", refs.selectedKey)
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

func TestInlineControlCommandAcceptsBooleanAliases(t *testing.T) {
	setupMonitorPipelineTestGraph()
	enableControlFile = false

	result := invokeInlineCommand("!!! control yes")
	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusApplied, result.Result)
	}
	if !enableControlFile {
		t.Fatal("control yes did not enable control file")
	}

	result = invokeInlineCommand("!!! control no")
	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusApplied, result.Result)
	}
	if enableControlFile {
		t.Fatal("control no did not disable control file")
	}
}

func TestInlineControlCommandRejectsInvalidBooleanValue(t *testing.T) {
	setupMonitorPipelineTestGraph()
	enableControlFile = false

	result := invokeInlineCommand("!!! control maybe")

	assertInlineInvalidArgument(t, result, "control", "maybe", "control requires a boolean value")
	if enableControlFile {
		t.Fatal("control maybe enabled control file")
	}
}

func TestInlineSettingsRejectInvalidArguments(t *testing.T) {
	setupMonitorPipelineTestGraph()
	PattyGraph.pattyConfig.mbToRead = DefaultMBToRead

	tests := []struct {
		command  string
		argument string
		value    string
		message  string
	}{
		{"!!! push nope", "push", "nope", "push requires an integer"},
		{"!!! push -1", "push", "-1", "push must be between 0 and 11"},
		{"!!! push 12", "push", "12", "push must be between 0 and 11"},
		{"!!! grace nope", "grace", "nope", "grace requires an integer"},
		{"!!! peak-limit nope", "peak-limit", "nope", "peak-limit requires an integer"},
		{"!!! flux nope", "flux", "nope", "flux requires an integer"},
		{"!!! flux 99", "flux", "99", "flux must be between 1 and 10"},
		{"!!! scale nope", "scale", "nope", "scale requires a number"},
		{"!!! scale NaN", "scale", "NaN", "scale must be a finite number"},
		{"!!! read nope", "read", "nope", "read requires an integer"},
		{"!!! read -1", "read", "-1", "read must be zero or greater"},
		{"!!! color-index nope", "color-index", "nope", "color-index requires an integer"},
		{
			fmt.Sprintf("!!! color-index %d", len(AutobotColors)),
			"color-index",
			fmt.Sprintf("%d", len(AutobotColors)),
			fmt.Sprintf("color-index must be between 0 and %d", len(AutobotColors)-1),
		},
	}

	for _, test := range tests {
		t.Run(test.command, func(t *testing.T) {
			result := invokeInlineCommand(test.command)
			assertInlineInvalidArgument(t, result, test.argument, test.value, test.message)
		})
	}

	if pattyPushFactor != pattyPushFactorDefault ||
		pattyGracePeriod != pattyGracePeriodDefault ||
		pattyScaleFactor != pattyScaleFactorDefault ||
		peakWordLimit != peakWordLimitDefault ||
		fluxDepth != DefaultFluxDepth ||
		colorIndex != 0 ||
		PattyGraph.pattyConfig.mbToRead != DefaultMBToRead {
		t.Fatal("invalid setting argument changed runtime state")
	}
}

func TestInlinePeakLimitClampsBounds(t *testing.T) {
	for _, test := range []struct {
		name      string
		requested int
		effective int
	}{
		{"below", 0, peakWordLimitMin},
		{"negative", -5, peakWordLimitMin},
		{"above", 30, peakWordLimitMax},
	} {
		t.Run(test.name, func(t *testing.T) {
			setupMonitorPipelineTestGraph()
			initialFacts := len(facts.forced)

			result := invokeInlineCommand(fmt.Sprintf("!!! peak-limit %d", test.requested))

			if result.Status != InlineCommandStatusApplied {
				t.Fatalf("status = %q, want applied: %#v", result.Status, result.Result)
			}
			if peakWordLimit != test.effective {
				t.Fatalf("peakWordLimit = %d, want %d", peakWordLimit, test.effective)
			}
			if result.Result["requested_value"] != test.requested || result.Result["effective_value"] != test.effective {
				t.Fatalf("requested/effective = %#v/%#v, want %d/%d", result.Result["requested_value"], result.Result["effective_value"], test.requested, test.effective)
			}
			if result.Result["value"] != strconv.Itoa(test.effective) || result.Result["clamped"] != true {
				t.Fatalf("clamped result = %#v", result.Result)
			}
			wantWarning := fmt.Sprintf("peak-limit clamped from %d to %d", test.requested, test.effective)
			if result.Result["warning"] != wantWarning {
				t.Fatalf("warning = %#v, want %q", result.Result["warning"], wantWarning)
			}

			foundWarning := false
			for _, fact := range facts.forced[initialFacts:] {
				if fact.Name != "settings.peak-limit-clamped" {
					continue
				}
				foundWarning = true
				wantText := fmt.Sprintf("Peak limit clamped:%d->%d", test.requested, test.effective)
				if text := stripBrackets(fact.Generate(nil)); !strings.Contains(text, wantText) {
					t.Fatalf("clamp fact = %q, want %q", text, wantText)
				}
			}
			if !foundWarning {
				t.Fatal("clamped setting did not queue named warning fact")
			}
		})
	}
}

func TestInlinePeakLimitAcceptsInRangeValue(t *testing.T) {
	setupMonitorPipelineTestGraph()

	result := invokeInlineCommand("!!! peak-limit 15")

	if result.Status != InlineCommandStatusApplied || peakWordLimit != 15 {
		t.Fatalf("result/limit = %#v/%d, want applied/15", result, peakWordLimit)
	}
	if result.Result["value"] != "15" {
		t.Fatalf("value = %#v, want 15", result.Result["value"])
	}
	for _, key := range []string{"requested_value", "effective_value", "clamped", "warning"} {
		if _, exists := result.Result[key]; exists {
			t.Fatalf("in-range result unexpectedly included %q: %#v", key, result.Result)
		}
	}
}

func TestInlinePushAcceptsZero(t *testing.T) {
	setupMonitorPipelineTestGraph()

	result := invokeInlineCommand("!!! push 0")

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusApplied, result.Result)
	}
	if pattyPushFactor != 0 {
		t.Fatalf("pattyPushFactor = %d, want 0", pattyPushFactor)
	}
	if result.Result["value"] != "0" {
		t.Fatalf("value = %v, want 0", result.Result["value"])
	}
}

func TestInlineColorIndexPushesRegisteredFactoid(t *testing.T) {
	setupMonitorPipelineTestGraph()
	facts.forced = nil

	result := invokeInlineCommand("!!! color-index 1")

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusApplied, result.Result)
	}
	message, _, factName := facts.Next()
	if factName != "settings.color-index" {
		t.Fatalf("fact name = %q, want settings.color-index", factName)
	}
	if !strings.Contains(message, "Next Color:") {
		t.Fatalf("fact message = %q, want Next Color", message)
	}
}

func TestInlinePropertyRejectsMalformedOrEmptyValue(t *testing.T) {
	setupMonitorPipelineTestGraph()

	result := invokeInlineCommand("!!! title '")
	assertInlineInvalidArgument(t, result, "title", "'", "unclosed quote in input")

	result = invokeInlineCommand("!!! title ''")
	assertInlineInvalidArgument(t, result, "title", "", "title requires a value")
}

func TestInlineControlFileRuntimeUpdatesConfigOnly(t *testing.T) {
	setupMonitorPipelineTestGraph()
	enableControlFile = false
	PattyGraph.pattyConfig.setSaveDir("/tmp/patty")

	result := invokeInlineCommand("!!! control-file current.control")

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q", result.Status, InlineCommandStatusApplied)
	}
	if enableControlFile {
		t.Fatal("enableControlFile = true after runtime control-file, want unchanged false")
	}
	if got, want := PattyGraph.pattyConfig.controlFile, "/tmp/patty/current.control"; got != want {
		t.Fatalf("controlFile = %q, want %q", got, want)
	}
	if result.Result["runtime_effect"] != "next_config_only" {
		t.Fatalf("runtime_effect = %v, want next_config_only", result.Result["runtime_effect"])
	}
}

func TestConfigInlineControlFileImpliesControl(t *testing.T) {
	setupMonitorPipelineTestGraph()
	enableControlFile = false
	PattyGraph.pattyConfig.setSaveDir("/tmp/patty")

	result := invokeInlineCommandWithOptions(
		"!!! control-file current.control",
		InlineCommandOptions{allowCreateSaveDir: true},
	)

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q", result.Status, InlineCommandStatusApplied)
	}
	if !enableControlFile {
		t.Fatal("enableControlFile = false after config control-file, want true")
	}
	if got, want := PattyGraph.pattyConfig.controlFile, "/tmp/patty/current.control"; got != want {
		t.Fatalf("controlFile = %q, want %q", got, want)
	}
}

func TestInlineSaveDirRejectsMissingDirectoryResult(t *testing.T) {
	silenceExpectedLogs(t)
	setupMonitorPipelineTestGraph()
	existing := t.TempDir()
	missing := filepath.Join(t.TempDir(), "missing")
	PattyGraph.pattyConfig.setSaveDir(existing)

	result := invokeInlineCommand("!!! save-dir " + missing)

	if result.Status != InlineCommandStatusRejected {
		t.Fatalf("status = %q, want %q", result.Status, InlineCommandStatusRejected)
	}
	if result.Result["name"] != "save-dir" {
		t.Fatalf("result name = %v, want save-dir", result.Result["name"])
	}
	if result.Result["path"] != missing {
		t.Fatalf("result path = %v, want %s", result.Result["path"], missing)
	}
	if got := PattyGraph.pattyConfig.saveDir; got != existing {
		t.Fatalf("saveDir = %q, want unchanged existing dir %q", got, existing)
	}
}

func TestConfigInlineSaveDirCanCreateMissingDirectory(t *testing.T) {
	setupMonitorPipelineTestGraph()
	missing := filepath.Join(t.TempDir(), "missing", "nested")

	result := invokeInlineCommandWithOptions(
		"!!! save-dir "+missing,
		InlineCommandOptions{allowCreateSaveDir: true},
	)

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q", result.Status, InlineCommandStatusApplied)
	}
	if got := PattyGraph.pattyConfig.saveDir; got != missing {
		t.Fatalf("saveDir = %q, want %q", got, missing)
	}
	if info, err := os.Stat(missing); err != nil {
		t.Fatalf("config save-dir was not created: %v", err)
	} else if !info.IsDir() {
		t.Fatalf("config save-dir path is not a directory")
	}
}

func TestInlineAlertSetShowAndRangeValidation(t *testing.T) {
	setupMonitorPipelineTestGraph()

	above := invokeInlineCommand("!!! alert errs above 50")
	if above.Status != InlineCommandStatusApplied {
		t.Fatalf("above status = %q, want applied: %#v", above.Status, above.Result)
	}
	if PattyGraph.errsMatcher.AlertAbove.Threshold != 50 {
		t.Fatalf("above threshold = %d, want 50", PattyGraph.errsMatcher.AlertAbove.Threshold)
	}

	below := invokeInlineCommand("!!! alert errs below 10")
	if below.Status != InlineCommandStatusApplied {
		t.Fatalf("below status = %q, want applied: %#v", below.Status, below.Result)
	}
	if PattyGraph.errsMatcher.AlertBelow.Threshold != 10 {
		t.Fatalf("below threshold = %d, want 10", PattyGraph.errsMatcher.AlertBelow.Threshold)
	}

	show := invokeInlineCommand("!!! alert errs")
	if show.Status != InlineCommandStatusApplied {
		t.Fatalf("show status = %q, want applied", show.Status)
	}
	if show.Result["action"] != "show_alert" {
		t.Fatalf("show action = %v, want show_alert", show.Result["action"])
	}
	if show.Result["matcher"] != "errs" {
		t.Fatalf("show matcher = %v, want errs", show.Result["matcher"])
	}

	overlap := invokeInlineCommand("!!! alert errs below 100")
	if overlap.Status != InlineCommandStatusRejected {
		t.Fatalf("overlap status = %q, want rejected", overlap.Status)
	}
	belowZero := invokeInlineCommand("!!! alert errs below 0")
	if belowZero.Status != InlineCommandStatusRejected {
		t.Fatalf("below zero status = %q, want rejected", belowZero.Status)
	}
}

func TestMatcherAlertsTriggerRecoverAndListOnConsecutiveFluxDepth(t *testing.T) {
	setupMonitorPipelineTestGraph()
	fluxDepth = 2
	invokeInlineCommand("!!! alert errs above 3")
	errs := PattyGraph.errsMatcher

	errs.intervalCount = 3
	errs.push()
	if len(PattyGraph.pendingAlertTransitions) != 0 {
		t.Fatalf("pending transitions after first hit = %d, want 0", len(PattyGraph.pendingAlertTransitions))
	}

	errs.intervalCount = 4
	errs.push()
	if len(PattyGraph.pendingAlertTransitions) != 1 {
		t.Fatalf("pending transitions after second hit = %d, want 1", len(PattyGraph.pendingAlertTransitions))
	}
	trigger := PattyGraph.pendingAlertTransitions[0]
	if trigger.Status != AlertStatusTriggered || trigger.Direction != AlertDirectionAbove || trigger.Value != 4 || trigger.Threshold != 3 {
		t.Fatalf("trigger transition = %#v", trigger)
	}

	alerts := invokeInlineCommand("!!! alerts")
	active, ok := alerts.Result["active_alerts"].([]map[string]interface{})
	if !ok || len(active) != 1 {
		t.Fatalf("active alerts = %#v, want one alert", alerts.Result["active_alerts"])
	}
	if active[0]["matcher"] != "errs" || active[0]["direction"] != AlertDirectionAbove {
		t.Fatalf("active alert = %#v", active[0])
	}

	PattyGraph.clearPendingAlertTransitions()
	errs.intervalCount = 10
	errs.push()
	if len(PattyGraph.pendingAlertTransitions) != 0 {
		t.Fatalf("pending transitions while still active = %d, want 0", len(PattyGraph.pendingAlertTransitions))
	}

	errs.intervalCount = 2
	errs.push()
	if len(PattyGraph.pendingAlertTransitions) != 0 {
		t.Fatalf("pending transitions after first clear = %d, want 0", len(PattyGraph.pendingAlertTransitions))
	}
	errs.intervalCount = 0
	errs.push()
	if len(PattyGraph.pendingAlertTransitions) != 1 {
		t.Fatalf("pending transitions after second clear = %d, want 1", len(PattyGraph.pendingAlertTransitions))
	}
	recovered := PattyGraph.pendingAlertTransitions[0]
	if recovered.Status != AlertStatusRecovered || recovered.Direction != AlertDirectionAbove || recovered.Value != 0 {
		t.Fatalf("recovered transition = %#v", recovered)
	}
}

func TestInlineAlertClearActiveBoundDoesNotEmitRecovered(t *testing.T) {
	setupMonitorPipelineTestGraph()
	fluxDepth = 1
	invokeInlineCommand("!!! alert errs below 1")
	errs := PattyGraph.errsMatcher
	errs.intervalCount = 0
	errs.push()
	if len(PattyGraph.pendingAlertTransitions) != 1 {
		t.Fatalf("pending transitions after trigger = %d, want 1", len(PattyGraph.pendingAlertTransitions))
	}
	PattyGraph.clearPendingAlertTransitions()

	clearResult := invokeInlineCommand("!!! alert errs clear below")
	if clearResult.Status != InlineCommandStatusApplied {
		t.Fatalf("clear status = %q, want applied", clearResult.Status)
	}
	if clearResult.Result["was_active"] != true {
		t.Fatalf("was_active = %v, want true", clearResult.Result["was_active"])
	}
	if PattyGraph.errsMatcher.AlertBelow.Enabled || PattyGraph.errsMatcher.AlertBelow.Active {
		t.Fatalf("below bound after clear = %#v", PattyGraph.errsMatcher.AlertBelow)
	}
	if len(PattyGraph.pendingAlertTransitions) != 0 {
		t.Fatalf("pending transitions after manual clear = %d, want 0", len(PattyGraph.pendingAlertTransitions))
	}
}

func TestFluxChangeResetsAlertRuntimeStateButKeepsBounds(t *testing.T) {
	setupMonitorPipelineTestGraph()
	fluxDepth = 1
	invokeInlineCommand("!!! alert errs above 1")
	errs := PattyGraph.errsMatcher
	errs.intervalCount = 2
	errs.push()
	if !errs.AlertAbove.Active {
		t.Fatal("above alert active = false, want true before flux change")
	}
	if len(PattyGraph.pendingAlertTransitions) != 1 {
		t.Fatalf("pending transitions = %d, want 1 before flux change", len(PattyGraph.pendingAlertTransitions))
	}

	if !setFlux(3) {
		t.Fatal("setFlux(3) = false, want true")
	}
	if !errs.AlertAbove.Enabled || errs.AlertAbove.Threshold != 1 {
		t.Fatalf("above bound after flux change = %#v, want enabled threshold 1", errs.AlertAbove)
	}
	if errs.AlertAbove.Active || errs.AlertAbove.HitRun != 0 || errs.AlertAbove.ClearRun != 0 || errs.AlertAbove.LastValue != 0 {
		t.Fatalf("above runtime state after flux change = %#v, want reset", errs.AlertAbove)
	}
	if len(PattyGraph.pendingAlertTransitions) != 0 {
		t.Fatalf("pending transitions after flux change = %d, want 0", len(PattyGraph.pendingAlertTransitions))
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

func TestInlineAddAcceptsFlagFirstScope(t *testing.T) {
	setupMonitorPipelineTestGraph()

	result := invokeInlineCommand("!!! add --refs refProbe filter")

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusApplied, result.Result)
	}
	if result.Result["matcher_name"] != "refProbe" {
		t.Fatalf("matcher_name = %v, want refProbe", result.Result["matcher_name"])
	}
	if result.Result["scope"] != "refs" {
		t.Fatalf("scope = %v, want refs", result.Result["scope"])
	}
	if matcherIndexByNameForTest("-refs") != -1 {
		t.Fatal("flag-first add created malformed -refs matcher")
	}
	if matcherIndexByNameForTest("refProbe") == -1 {
		t.Fatal("refProbe matcher was not added")
	}
	patterns, ok := result.Result["patterns"].([]string)
	if !ok || len(patterns) != 1 || patterns[0] != "filter" {
		t.Fatalf("patterns = %#v, want [filter]", result.Result["patterns"])
	}
}

func TestInlineAddAcceptsFlagFirstScopeWithPlacement(t *testing.T) {
	setupMonitorPipelineTestGraph()

	result := invokeInlineCommand("!!! add --refs *observerProbe filter")

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusApplied, result.Result)
	}
	if result.Result["matcher_name"] != "observerProbe" {
		t.Fatalf("matcher_name = %v, want observerProbe", result.Result["matcher_name"])
	}
	if result.Result["placement"] != "before_lines" {
		t.Fatalf("placement = %v, want before_lines", result.Result["placement"])
	}
	observerIndex := matcherIndexByNameForTest("observerProbe")
	linesIndex := matcherIndexByNameForTest("lines")
	if observerIndex == -1 {
		t.Fatal("observerProbe matcher was not added")
	}
	if observerIndex <= botsIndex || observerIndex >= linesIndex {
		t.Fatalf("observerProbe index = %d, want below Bots and before lines", observerIndex)
	}
}

func TestInlineAddRejectsScopeFlagWithoutMatcherName(t *testing.T) {
	setupMonitorPipelineTestGraph()

	result := invokeInlineCommand("!!! add --words")

	if result.Status != InlineCommandStatusRejected {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusRejected, result.Result)
	}
	if result.Result["error"] != "missing matcher name" {
		t.Fatalf("error = %v, want missing matcher name", result.Result["error"])
	}
}

func TestInlineAddRejectsFlagShapedMatcherName(t *testing.T) {
	setupMonitorPipelineTestGraph()

	result := invokeInlineCommand("!!! add --unknown pattern")

	if result.Status != InlineCommandStatusRejected {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusRejected, result.Result)
	}
	if result.Result["error"] != "matcher name cannot look like a flag" {
		t.Fatalf("error = %v, want matcher name cannot look like a flag", result.Result["error"])
	}
	if result.Result["raw_matcher_name"] != "--unknown" {
		t.Fatalf("raw_matcher_name = %v, want --unknown", result.Result["raw_matcher_name"])
	}
}

func TestInlineAddRejectsMatcherNameWithLiteralPlacementPrefix(t *testing.T) {
	setupMonitorPipelineTestGraph()

	for _, command := range []string{
		"!!! add ++badName pattern",
		"!!! add *+badName pattern",
		"!!! add -+badName pattern",
	} {
		result := invokeInlineCommand(command)
		if result.Status != InlineCommandStatusRejected {
			t.Fatalf("%s status = %q, want %q: %#v", command, result.Status, InlineCommandStatusRejected, result.Result)
		}
		if result.Result["error"] != "matcher name cannot begin with a placement prefix" {
			t.Fatalf("%s error = %v, want matcher name cannot begin with a placement prefix", command, result.Result["error"])
		}
		if result.Result["raw_matcher_name"] == "" {
			t.Fatalf("%s raw_matcher_name was empty", command)
		}
		if result.Result["normalized_matcher_name"] == "" {
			t.Fatalf("%s normalized_matcher_name was empty", command)
		}
	}
}

func TestInlineAddRejectsEmptyNameAfterPlacementPrefix(t *testing.T) {
	setupMonitorPipelineTestGraph()

	result := invokeInlineCommand("!!! add + pattern")

	if result.Status != InlineCommandStatusRejected {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusRejected, result.Result)
	}
	if result.Result["error"] != "matcher name is empty after placement prefix" {
		t.Fatalf("error = %v, want matcher name is empty after placement prefix", result.Result["error"])
	}
	if result.Result["raw_matcher_name"] != "+" {
		t.Fatalf("raw_matcher_name = %v, want +", result.Result["raw_matcher_name"])
	}
}

func TestInlineAddAcceptsFlagFirstRegex(t *testing.T) {
	setupMonitorPipelineTestGraph()

	result := invokeInlineCommand("!!! add --regex botProbe Googlebot|bingbot")

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusApplied, result.Result)
	}
	if result.Result["matcher_name"] != "botProbe" {
		t.Fatalf("matcher_name = %v, want botProbe", result.Result["matcher_name"])
	}
	if result.Result["regex"] != true {
		t.Fatalf("regex = %v, want true", result.Result["regex"])
	}
	if result.Result["scope"] != "regex" {
		t.Fatalf("scope = %v, want regex", result.Result["scope"])
	}
}

func TestInlineAddRejectsInvalidRegexWithCompilerDetail(t *testing.T) {
	setupMonitorPipelineTestGraph()

	for _, command := range []string{
		"!!! add regexProbe --regex '[invalid'",
		"!!! add --regex regexProbe '[invalid'",
	} {
		result := invokeInlineCommand(command)
		if result.Status != InlineCommandStatusRejected {
			t.Fatalf("%s status = %q, want %q: %#v", command, result.Status, InlineCommandStatusRejected, result.Result)
		}
		if result.Result["error_kind"] != "invalid_argument" {
			t.Fatalf("%s error_kind = %v, want invalid_argument", command, result.Result["error_kind"])
		}
		if result.Result["argument"] != "regex" || result.Result["value"] != "[invalid" {
			t.Fatalf("%s regex fields = %#v", command, result.Result)
		}
		errorMessage, ok := result.Result["error"].(string)
		if !ok || !strings.Contains(errorMessage, "invalid regex pattern:") || !strings.Contains(errorMessage, "missing closing ]") {
			t.Fatalf("%s error = %v, want regex compiler detail", command, result.Result["error"])
		}
		if result.Result["matcher_name"] != "regexProbe" {
			t.Fatalf("%s matcher_name = %v, want regexProbe", command, result.Result["matcher_name"])
		}
		if matcherNameExists("regexProbe") {
			t.Fatalf("%s added matcher after regex rejection", command)
		}
	}
}

func TestInlineFactRejectsUnknownFactName(t *testing.T) {
	setupMonitorPipelineTestGraph()
	facts = NewFactoidGenerator()
	facts.forced = nil

	result := invokeInlineCommand("!!! fact no.such.fact")

	if result.Status != InlineCommandStatusRejected {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusRejected, result.Result)
	}
	if result.Result["error"] != "fact not found" {
		t.Fatalf("error = %v, want fact not found", result.Result["error"])
	}
	if result.Result["fact"] != "no.such.fact" {
		t.Fatalf("fact = %v, want no.such.fact", result.Result["fact"])
	}
	msg, _, _ := facts.Next()
	if !strings.Contains(msg, "Factoid not found: no.such.fact") {
		t.Fatalf("forced factoid = %q, want missing factoid error", msg)
	}
}

func TestInlineFactAcceptsKnownFactName(t *testing.T) {
	setupMonitorPipelineTestGraph()
	facts = NewFactoidGenerator()
	facts.forced = nil
	generateSidecarJSONL = true
	enableControlFile = true
	PattyGraph.pattyConfig.setSaveDir("/tmp/patty-splats")
	PattyGraph.pattyConfig.setJSONFile("current.jsonl")
	PattyGraph.pattyConfig.setControlFile("current.control")

	result := invokeInlineCommand("!!! fact output.paths")

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusApplied, result.Result)
	}
	if result.Result["fact"] != "output.paths" {
		t.Fatalf("fact = %v, want output.paths", result.Result["fact"])
	}
	text, ok := result.Result["text"].(string)
	if !ok {
		t.Fatalf("fact text = %#v, want string", result.Result["text"])
	}
	if !strings.Contains(text, "current.jsonl") || !strings.Contains(text, "current.control") {
		t.Fatalf("fact text = %q, want output filenames", text)
	}
	if strings.Contains(text, "[") || strings.Contains(text, "]") {
		t.Fatalf("fact text = %q, want JSONL text without display markup", text)
	}
	msg, _, _ := facts.Next()
	if !strings.Contains(msg, "current.jsonl") || !strings.Contains(msg, "current.control") {
		t.Fatalf("forced factoid = %q, want output filenames", msg)
	}
	if !strings.Contains(msg, "patty-splats") {
		t.Fatalf("forced factoid = %q, want save-dir name", msg)
	}
	if strings.Contains(msg, "json:on") || strings.Contains(msg, "control:on") {
		t.Fatalf("forced factoid = %q, want filenames instead of on", msg)
	}
}

func TestInlineFactPrintPreservesRawMessage(t *testing.T) {
	setupMonitorPipelineTestGraph()
	facts.forced = nil
	command := `!!! fact print "Oh  boy" # this is important`

	result := invokeInlineCommand(command)

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusApplied, result.Result)
	}
	if result.Result["fact"] != "print" {
		t.Fatalf("fact = %v, want print", result.Result["fact"])
	}
	if result.Result["text"] != `Note: "Oh  boy" # this is important` {
		t.Fatalf("text = %q, want raw message content", result.Result["text"])
	}
	text, rank, name := facts.Next()
	if name != "print" || rank != 100 {
		t.Fatalf("queued fact = %q rank %d, want print rank 100", name, rank)
	}
	for _, expected := range []string{
		internalFmt("Note:"),
		`[white] "Oh  boy" # this is important`,
		"[-:-:-:-]",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("ticker text = %q, want %q", text, expected)
		}
	}
}

func TestInlineFactPrintAcceptsTviewMarkupAndReturnsPlainText(t *testing.T) {
	setupMonitorPipelineTestGraph()
	facts.forced = nil

	result := invokeInlineCommand("!!! fact PRINT [red]urgent[white] now")

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want applied: %#v", result.Status, result.Result)
	}
	if result.Result["text"] != "Note: urgent now" {
		t.Fatalf("text = %q, want markup-free result", result.Result["text"])
	}
	text, _, _ := facts.Next()
	if !strings.Contains(text, "[red]urgent[white] now") || !strings.HasSuffix(text, "[-:-:-:-]") {
		t.Fatalf("ticker text = %q, want supplied markup and full reset", text)
	}
}

func TestInlineFactPrintRejectsInvalidMessages(t *testing.T) {
	tests := []struct {
		name    string
		message string
		error   string
	}{
		{name: "missing", message: "", error: "fact print requires message text"},
		{name: "whitespace", message: "   ", error: "fact print requires message text"},
		{name: "tags only", message: "[red]", error: "fact print requires visible message text"},
		{name: "control", message: "hello\x1bworld", error: "fact print message cannot contain control characters"},
		{name: "invalid utf8", message: string([]byte{'h', 0xff}), error: "fact print requires valid UTF-8 text"},
		{name: "byte limit", message: strings.Repeat("a", inlinePrintMaxBytes+1), error: "fact print message must be 1024 bytes or fewer"},
		{name: "visible limit", message: strings.Repeat("a", inlinePrintMaxVisible+1), error: "fact print message must be 256 visible characters or fewer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupMonitorPipelineTestGraph()
			facts.forced = nil
			result := invokeInlineCommand("!!! fact print " + tt.message)
			if result.Status != InlineCommandStatusRejected {
				t.Fatalf("status = %q, want rejected: %#v", result.Status, result.Result)
			}
			if result.Result["error_kind"] != "invalid_argument" || result.Result["argument"] != "message" {
				t.Fatalf("invalid argument fields = %#v", result.Result)
			}
			if result.Result["error"] != tt.error {
				t.Fatalf("error = %q, want %q", result.Result["error"], tt.error)
			}
			if len(facts.forced) != 0 {
				t.Fatalf("rejected message queued %d factoids", len(facts.forced))
			}
		})
	}
}

func TestInlineModeRejectsInvalidModeRange(t *testing.T) {
	setupMonitorPipelineTestGraph()
	invokeInlineCommand("!!! add modeProbe")

	result := invokeInlineCommand("!!! mode modeProbe 9")

	assertInlineInvalidArgument(t, result, "mode", "9", "mode must be between 0 and 2")
	if result.Result["matcher_name"] != "modeProbe" {
		t.Fatalf("matcher_name = %v, want modeProbe", result.Result["matcher_name"])
	}
	if result.Result["mode"] != 9 {
		t.Fatalf("mode = %v, want 9", result.Result["mode"])
	}
}

func TestInlineModeRejectsNegativeMode(t *testing.T) {
	setupMonitorPipelineTestGraph()
	invokeInlineCommand("!!! add modeProbe")

	result := invokeInlineCommand("!!! mode modeProbe -1")

	assertInlineInvalidArgument(t, result, "mode", "-1", "mode must be between 0 and 2")
}

func TestInlineModeRejectsMissingMatcher(t *testing.T) {
	setupMonitorPipelineTestGraph()

	result := invokeInlineCommand("!!! mode noSuchMatcher 1")

	if result.Status != InlineCommandStatusRejected {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusRejected, result.Result)
	}
	if result.Result["error"] != "matcher not found" {
		t.Fatalf("error = %v, want matcher not found", result.Result["error"])
	}
	if result.Result["matcher_name"] != "noSuchMatcher" {
		t.Fatalf("matcher_name = %v, want noSuchMatcher", result.Result["matcher_name"])
	}
}

func TestInlineModeAcceptsValidMode(t *testing.T) {
	setupMonitorPipelineTestGraph()
	invokeInlineCommand("!!! add modeProbe")

	result := invokeInlineCommand("!!! mode modeProbe 2")

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusApplied, result.Result)
	}
	if result.Result["mode"] != 2 {
		t.Fatalf("mode = %v, want 2", result.Result["mode"])
	}
	matcher := findMatcherByName("modeProbe")
	if matcher == nil {
		t.Fatal("modeProbe matcher was not found")
	}
	if matcher.displayMatchMode != 2 {
		t.Fatalf("displayMatchMode = %d, want 2", matcher.displayMatchMode)
	}
}

func TestInlineModeRejectsExtraArguments(t *testing.T) {
	setupMonitorPipelineTestGraph()
	invokeInlineCommand("!!! add modeProbe")

	result := invokeInlineCommand("!!! mode modeProbe 2 extra")

	if result.Status != InlineCommandStatusRejected {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusRejected, result.Result)
	}
	if result.Result["error"] != "unexpected extra arguments" {
		t.Fatalf("error = %v, want unexpected extra arguments", result.Result["error"])
	}
}

func TestInlineModeAllowsTrailingComment(t *testing.T) {
	setupMonitorPipelineTestGraph()
	invokeInlineCommand("!!! add modeProbe")

	result := invokeInlineCommand("!!! mode modeProbe 2 # useful note")

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusApplied, result.Result)
	}
	matcher := findMatcherByName("modeProbe")
	if matcher == nil {
		t.Fatal("modeProbe matcher was not found")
	}
	if matcher.displayMatchMode != 2 {
		t.Fatalf("displayMatchMode = %d, want 2", matcher.displayMatchMode)
	}
}

func TestInlineColorRejectsMissingMatcher(t *testing.T) {
	setupMonitorPipelineTestGraph()

	result := invokeInlineCommand("!!! color noSuchMatcher red")

	if result.Status != InlineCommandStatusRejected {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusRejected, result.Result)
	}
	if result.Result["error"] != "matcher not found" {
		t.Fatalf("error = %v, want matcher not found", result.Result["error"])
	}
	if result.Result["matcher_name"] != "noSuchMatcher" {
		t.Fatalf("matcher_name = %v, want noSuchMatcher", result.Result["matcher_name"])
	}
}

func TestInlineColorAppliesToExistingMatcher(t *testing.T) {
	setupMonitorPipelineTestGraph()
	invokeInlineCommand("!!! add colorProbe")

	result := invokeInlineCommand("!!! color colorProbe red")

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusApplied, result.Result)
	}
	if result.Result["color"] != "[red]" {
		t.Fatalf("color = %v, want [red]", result.Result["color"])
	}
	matcher := findMatcherByName("colorProbe")
	if matcher == nil {
		t.Fatal("colorProbe matcher was not found")
	}
	if matcher.color != "[red]" {
		t.Fatalf("matcher color = %q, want [red]", matcher.color)
	}
}

func TestInlineColorRejectsExtraArgumentsWithoutChangingColor(t *testing.T) {
	setupMonitorPipelineTestGraph()
	invokeInlineCommand("!!! add colorProbe")
	matcher := findMatcherByName("colorProbe")
	if matcher == nil {
		t.Fatal("colorProbe matcher was not found")
	}
	originalColor := matcher.color

	result := invokeInlineCommand("!!! color colorProbe red unintended-extra")

	assertInlineInvalidArgument(
		t,
		result,
		"extra_args",
		"unintended-extra",
		"color accepts a matcher name and one color",
	)
	if matcher.color != originalColor {
		t.Fatalf("matcher color = %q, want unchanged %q", matcher.color, originalColor)
	}
	if extras, ok := result.Result["extra_args"].([]string); !ok || strings.Join(extras, " ") != "unintended-extra" {
		t.Fatalf("extra_args = %#v, want [unintended-extra]", result.Result["extra_args"])
	}
}

func TestInlineColorAllowsTrailingComment(t *testing.T) {
	setupMonitorPipelineTestGraph()
	invokeInlineCommand("!!! add colorProbe")

	result := invokeInlineCommand("!!! color colorProbe #FF0000 # useful note")

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusApplied, result.Result)
	}
	matcher := findMatcherByName("colorProbe")
	if matcher == nil || matcher.color != "[#FF0000]" {
		t.Fatalf("matcher color = %v, want [#FF0000]", matcher)
	}
}

func TestInlineColorRejectsInvalidColorIndex(t *testing.T) {
	setupMonitorPipelineTestGraph()
	invokeInlineCommand("!!! add colorProbe")

	result := invokeInlineCommand("!!! color colorProbe 9999")

	assertInlineInvalidArgument(
		t,
		result,
		"color",
		"9999",
		fmt.Sprintf("color index must be between 0 and %d", len(AutobotColors)-1),
	)
	if result.Result["color"] != "9999" {
		t.Fatalf("color = %v, want 9999", result.Result["color"])
	}
}

func TestInlineColorAcceptsValidColorIndex(t *testing.T) {
	setupMonitorPipelineTestGraph()
	invokeInlineCommand("!!! add colorProbe")

	result := invokeInlineCommand("!!! color colorProbe 0")

	if result.Status != InlineCommandStatusApplied {
		t.Fatalf("status = %q, want %q: %#v", result.Status, InlineCommandStatusApplied, result.Result)
	}
	if result.Result["color"] != AutobotColors[0] {
		t.Fatalf("color = %v, want %s", result.Result["color"], AutobotColors[0])
	}
	matcher := findMatcherByName("colorProbe")
	if matcher == nil {
		t.Fatal("colorProbe matcher was not found")
	}
	if matcher.color != AutobotColors[0] {
		t.Fatalf("matcher color = %q, want %s", matcher.color, AutobotColors[0])
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
