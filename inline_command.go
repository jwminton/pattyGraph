// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Inline commands are PattyGraph's shared configuration and control language.
// Startup config files, control-file input, and log lines prefixed with
// InlinePreamble all converge here so matcher setup, runtime setting changes,
// and live actions behave the same no matter which surface delivered them.
// Generated config files are replayable command streams, not a separate config
// format.

const (
	InlineCommandStatusApplied  = "applied"
	InlineCommandStatusIgnored  = "ignored"
	InlineCommandStatusRejected = "rejected"
	InlineCommandStatusError    = "error"
)

type InlineCommandResult struct {
	CommandName string
	Status      string
	Result      map[string]interface{}
}

func inlineCommandResult(commandName string, status string, action string) InlineCommandResult {
	return InlineCommandResult{
		CommandName: strings.ToLower(commandName),
		Status:      status,
		Result:      map[string]interface{}{"action": action},
	}
}

func inlineCommandRejected(commandName string, action string, message string) InlineCommandResult {
	result := inlineCommandResult(commandName, InlineCommandStatusRejected, action)
	result.Result["error"] = message
	return result
}

func inlineCommandInvalidArgument(commandName, action, argument, value, message string) InlineCommandResult {
	result := inlineCommandRejected(commandName, action, message)
	result.Result["error_kind"] = "invalid_argument"
	result.Result["argument"] = argument
	result.Result["value"] = value
	return result
}

type InlineCommandOptions struct {
	allowCreateSaveDir bool
}

func inlineAddScopeFlag(value string) string {
	switch value {
	case "--words":
		return "words"
	case "--refs":
		return "refs"
	case "--ips":
		return "ips"
	case "--code":
		return "code"
	case "--line":
		return "line"
	case "--regex":
		return "regex"
	default:
		return ""
	}
}

func inlineAddPatterns(args []string, start int) []string {
	return inlineArgsBeforeComment(args[start:])
}

// inlineArgsBeforeComment applies the inline language's trailing-comment rule
// to an already tokenized argument tail. Callers must first consume required
// values that may legally begin with '#', notably hexadecimal matcher colors.
func inlineArgsBeforeComment(args []string) []string {
	out := []string{}
	for _, arg := range args {
		if strings.HasPrefix(arg, "#") {
			break
		}
		out = append(out, arg)
	}
	return out
}

func normalizeMatcherCommandName(name string) string {
	if name == "" {
		return name
	}
	if name[0:1] == "*" || name[0:1] == "+" || name[0:1] == "-" {
		return name[1:]
	}
	return name
}

func inlineBoolValue(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "on", "true", "1", "yes":
		return "on", true
	case "off", "false", "0", "no":
		return "off", true
	default:
		return "", false
	}
}

func validateInlineSettingArgument(name, value string) string {
	switch strings.ToLower(name) {
	case "push":
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return "push requires an integer"
		}
		if parsed < 0 || parsed > 11 {
			return "push must be between 0 and 11"
		}
	case "grace":
		if _, err := strconv.Atoi(value); err != nil {
			return "grace requires an integer"
		}
	case "flux":
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return "flux requires an integer"
		}
		if parsed < 1 || parsed > 10 {
			return "flux must be between 1 and 10"
		}
	case "scale":
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return "scale requires a number"
		}
		if math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return "scale must be a finite number"
		}
	case "read":
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return "read requires an integer"
		}
		if parsed < 0 {
			return "read must be zero or greater"
		}
	case "color-index":
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return "color-index requires an integer"
		}
		if parsed < 0 || parsed >= len(AutobotColors) {
			return fmt.Sprintf("color-index must be between 0 and %d", len(AutobotColors)-1)
		}
	}
	return ""
}

// invokeInlineCommand applies one command after its source has already been
// accepted as command input. Runtime callers that can overlap the tailer, TUI,
// or control file monitor must hold mu before invoking it; startup config replay
// is single-threaded and intentionally calls it without the runtime lock.
func invokeInlineCommand(line string) InlineCommandResult {
	return invokeInlineCommandWithOptions(line, InlineCommandOptions{})
}

func invokeInlineCommandWithOptions(line string, opts InlineCommandOptions) InlineCommandResult {
	// Assume '!!! ' prefix already detected
	commandLine := line[4:]
	// Inline commands share dispatch but intentionally own their argument grammar:
	// strings.Fields identifies the command and supports hard-stop commands such as
	// del and purge; quote-aware commands use splitArgsShellStyle; callers opt into
	// trailing '#' comments after consuming any required hash-prefixed values. Mode
	// and color enforce fixed arity, while property and no-argument commands stop
	// processing once their documented inputs are satisfied.
	tokens := strings.Fields(commandLine)
	if len(tokens) == 0 {
		return inlineCommandResult("", InlineCommandStatusIgnored, "empty")
	}

	cmd := tokens[0]

	switch cmd {
	// === Matchers Management ===
	case "ADD", "add":
		// add uses quote-aware parsing because patterns may contain spaces; scope
		// flags are accepted before or after the one-token matcher name.
		args, err := splitArgsShellStyle(commandLine[len(cmd):])
		if err != nil || len(args) < 1 {
			return inlineCommandRejected(cmd, "add_matcher", "missing matcher name")
		}

		isRegex := false
		scopeName := "line"
		name := args[0]
		patternStart := 1

		if inlineAddScopeFlag(args[0]) != "" {
			if len(args) < 2 {
				return inlineCommandRejected(cmd, "add_matcher", "missing matcher name")
			}
			scopeName = inlineAddScopeFlag(args[0])
			isRegex = args[0] == "--regex"
			name = args[1]
			patternStart = 2
		} else if len(args) > 1 && inlineAddScopeFlag(args[1]) != "" {
			scopeName = inlineAddScopeFlag(args[1])
			isRegex = args[1] == "--regex"
			patternStart = 2
		} else if strings.HasPrefix(args[0], "--") {
			result := inlineCommandRejected(cmd, "add_matcher", "matcher name cannot look like a flag")
			result.Result["raw_matcher_name"] = args[0]
			return result
		}

		patterns := inlineAddPatterns(args, patternStart)

		originalName := name
		if name[0:1] == "*" {
			name = name[1:]
		} else if name[0:1] == "+" {
			name = name[1:]
		} else if name[0:1] == "-" {
			name = name[1:]
		}
		if name == "" {
			result := inlineCommandRejected(cmd, "add_matcher", "matcher name is empty after placement prefix")
			result.Result["raw_matcher_name"] = originalName
			return result
		}
		if strings.HasPrefix(name, "--") {
			result := inlineCommandRejected(cmd, "add_matcher", "matcher name cannot look like a flag")
			result.Result["raw_matcher_name"] = originalName
			result.Result["normalized_matcher_name"] = name
			return result
		}
		if name[0:1] == "*" || name[0:1] == "+" || name[0:1] == "-" {
			result := inlineCommandRejected(cmd, "add_matcher", "matcher name cannot begin with a placement prefix")
			result.Result["raw_matcher_name"] = originalName
			result.Result["normalized_matcher_name"] = name
			return result
		}
		if strings.IndexFunc(name, unicode.IsSpace) >= 0 {
			result := inlineCommandInvalidArgument(cmd, "add_matcher", "matcher_name", name, "matcher name cannot contain whitespace")
			result.Result["raw_matcher_name"] = originalName
			result.Result["normalized_matcher_name"] = name
			return result
		}
		if name == BotsMatcherName {
			toggleBotsMatcher(false)
			result := inlineCommandResult(cmd, InlineCommandStatusApplied, "enable_bots_auto_add")
			result.Result["matcher_name"] = name
			return result
		}

		if len(patterns) == 0 {
			if name == "Browser" || name == "Platform" {
				isRegex = true
			} else {
				patterns = []string{name}
			}
		}
		if matcherNameExists(name) {
			return inlineCommandRejected(cmd, "add_matcher", "duplicate matcher name")
		}
		//newM := PattyGraph.createMatcher(name, isLikelyIPPattern(name), patterns)
		var newM *Matcher
		if isRegex {
			if name == "Browser" && len(patterns) == 0 {
				newM = newRegexMatcher(name, browserRegexString)
			} else if name == "Platform" && len(patterns) == 0 {
				newM = newRegexMatcher(name, platformRegexString)
			} else {
				pattern := strings.Join(patterns, " ")
				if _, err := regexp.Compile(pattern); err != nil {
					result := inlineCommandInvalidArgument(
						cmd,
						"add_matcher",
						"regex",
						pattern,
						fmt.Sprintf("invalid regex pattern: %v", err),
					)
					result.Result["matcher_name"] = name
					return result
				}
				newM = newRegexMatcher(name, pattern)
			}
		} else {
			switch scopeName {
			case "words":
				newM = WordsMatcher(name, patterns)
			case "refs":
				newM = RefsMatcher(name, patterns)
			case "ips":
				newM = IpsMatcher(name, patterns)
			case "code":
				newM = CodeMatcher(name, patterns)
			default:
				newM = PattyGraph.createMatcher(name, areLikelyIPPatterns(patterns), patterns)
			}
		}
		if newM == nil {
			return inlineCommandRejected(cmd, "add_matcher", "matcher was not created")
		}
		newM.inlineCommandAction = func() string {
			return line
		}
		placement := "before_bots"
		// Name prefixes choose the matcher lane: + goes first in the
		// competitive lane, -/default goes immediately before Bots, and * goes
		// below Bots before system rows so it observes instead of competes.
		switch originalName[0:1] {
		case "*":
			placement = "before_lines"
			PattyGraph.matchers = insertMatcherBeforeLines(PattyGraph.matchers, newM)
		case "+":
			placement = "first"
			PattyGraph.matchers = insertMatcherFirst(PattyGraph.matchers, newM)
		case "-":
			placement = "before_bots"
			PattyGraph.matchers = insertMatcherBeforeBots(PattyGraph.matchers, newM)
		default:
			if len(newM.history) > 0 {
				placement = "first"
				PattyGraph.matchers = insertMatcherFirst(PattyGraph.matchers, newM)
			} else {
				PattyGraph.matchers = insertMatcherBeforeBots(PattyGraph.matchers, newM)
			}
		}
		botsIndex = botsMatcherIndex()
		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "add_matcher")
		result.Result["matcher_name"] = name
		result.Result["placement"] = placement
		result.Result["scope"] = scopeName
		result.Result["patterns"] = patterns
		result.Result["regex"] = isRegex
		return result
	case "select", "SELECT":
		args, err := splitArgsShellStyle(commandLine[len(cmd):])
		if err != nil {
			return inlineCommandRejected(cmd, "select_interesting", "missing scope or selection")
		}
		if len(args) == 0 {
			if PattyGraph.selectedInterestingMatcher != nil {
				PattyGraph.selectedInterestingMatcher.selectedKey = ""
				PattyGraph.selectedInterestingMatcher.selectedGraphCache = ""
				PattyGraph.selectedInterestingMatcher = nil
			}
			return inlineCommandResult(cmd, InlineCommandStatusApplied, "clear_selection")
		}
		if len(args) < 2 {
			return inlineCommandRejected(cmd, "select_interesting", "missing selection key")
		}

		scope := strings.ToLower(strings.TrimLeft(args[0], "-"))
		selection := strings.TrimSpace(strings.Join(args[1:], " "))

		var target *InterestingWordMatcher
		switch scope {
		case "words":
			target = PattyGraph.wordsMatcher
		case "refs":
			target = PattyGraph.refsMatcher
		case "ips":
			target = PattyGraph.ipsMatcher
		default:
			return inlineCommandRejected(cmd, "select_interesting", "unsupported selection scope")
		}

		if target == nil {
			return inlineCommandRejected(cmd, "select_interesting", "selection target unavailable")
		}

		idx, ok := target.selectDisplayItemByKey(selection)
		if !ok {
			return inlineCommandRejected(cmd, "select_interesting", "selection not found")
		}

		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "select_interesting")
		result.Result["matcher"] = target.mName
		result.Result["selection"] = target.selectionKey()
		result.Result["selection_index"] = idx
		return result
	case "DEL", "del", "delete", "DELETE":
		if len(tokens) < 2 {
			return inlineCommandRejected(cmd, "delete_matcher", "missing matcher name")
		}
		fromTop := true
		name := tokens[1]
		// del intentionally consumes only the matcher token. This lets an operator
		// turn an add declaration into a delete by changing the command word while
		// leaving placement and pattern text in place.
		if name[0:1] == "*" {
			name = name[1:]
		}
		if name[0:1] == "+" {
			name = name[1:]
		} else if name[0:1] == "-" {
			name = name[1:]
			fromTop = false
		}
		matcher := findMatcherByName(name)
		if matcher == nil {
			result := inlineCommandRejected(cmd, "delete_matcher", "matcher not found")
			result.Result["matcher_name"] = name
			result.Result["from_top"] = fromTop
			return result
		}
		if matcher == PattyGraph.botsMatcher {
			toggleBotsMatcher(true)
			result := inlineCommandResult(cmd, InlineCommandStatusApplied, "disable_bots_auto_add")
			result.Result["matcher_name"] = name
			result.Result["from_top"] = fromTop
			return result
		}
		if matcher == PattyGraph.bytesMatcher ||
			matcher == PattyGraph.linesMatcher ||
			matcher == PattyGraph.errsMatcher {
			result := inlineCommandRejected(cmd, "delete_matcher", "matcher cannot be deleted")
			result.Result["matcher_name"] = name
			result.Result["from_top"] = fromTop
			return result
		}
		removeMatcherFromTop(name, fromTop)
		botsIndex = botsMatcherIndex()
		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "delete_matcher")
		result.Result["matcher_name"] = name
		result.Result["from_top"] = fromTop
		return result

	case "mode", "MODE":
		if len(tokens) < 2 {
			return inlineCommandRejected(cmd, "set_matcher_mode", "missing matcher name")
		}
		name := tokens[1]
		args, err := splitArgsShellStyle(commandLine[len(cmd):])
		if err != nil || len(args) < 2 {
			return inlineCommandRejected(cmd, "set_matcher_mode", "missing mode")
		}
		args = inlineArgsBeforeComment(args)
		if len(args) > 2 {
			result := inlineCommandRejected(cmd, "set_matcher_mode", "unexpected extra arguments")
			result.Result["matcher_name"] = name
			result.Result["extra_args"] = args[2:]
			return result
		}
		newMode, e := strconv.Atoi(args[1])
		if e != nil {
			return inlineCommandInvalidArgument(cmd, "set_matcher_mode", "mode", args[1], "mode requires an integer")
		}
		// Placement prefixes are accepted on matcher-management commands so a
		// copied add declaration can be edited without first cleaning its name.
		if name[0:1] == "*" {
			name = name[1:]
		}
		if name[0:1] == "+" {
			name = name[1:]
		} else if name[0:1] == "-" {
			name = name[1:]
		}
		if newMode < 0 || newMode > 2 {
			result := inlineCommandInvalidArgument(cmd, "set_matcher_mode", "mode", args[1], "mode must be between 0 and 2")
			result.Result["matcher_name"] = name
			result.Result["mode"] = newMode
			return result
		}
		if !matcherNameExists(name) {
			result := inlineCommandRejected(cmd, "set_matcher_mode", "matcher not found")
			result.Result["matcher_name"] = name
			result.Result["mode"] = newMode
			return result
		}
		setMatcherMode(name, newMode)
		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "set_matcher_mode")
		result.Result["matcher_name"] = name
		result.Result["mode"] = newMode
		return result

	case "COLOR", "color":
		args, err := splitArgsShellStyle(commandLine[len(cmd):])
		if err != nil {
			return inlineCommandInvalidArgument(cmd, "set_matcher_color", "arguments", strings.TrimSpace(commandLine[len(cmd):]), err.Error())
		}
		if len(args) < 2 {
			log.Printf("Invalid SET_COLOR usage: %s", commandLine)
			return inlineCommandRejected(cmd, "set_matcher_color", "missing matcher name or color")
		}
		// The color itself may be an unquoted #RRGGBB value. Apply comment
		// handling only to tokens after the required name and color.
		extraArgs := inlineArgsBeforeComment(args[2:])
		if len(extraArgs) > 0 {
			result := inlineCommandInvalidArgument(
				cmd,
				"set_matcher_color",
				"extra_args",
				strings.Join(extraArgs, " "),
				"color accepts a matcher name and one color",
			)
			result.Result["matcher_name"] = normalizeMatcherCommandName(args[0])
			result.Result["extra_args"] = extraArgs
			return result
		}
		name := args[0]
		color := args[1]
		cleanName := normalizeMatcherCommandName(name)
		if !matcherNameExists(cleanName) {
			result := inlineCommandRejected(cmd, "set_matcher_color", "matcher not found")
			result.Result["matcher_name"] = cleanName
			result.Result["color"] = color
			return result
		}
		if newIndex, err := strconv.Atoi(color); err == nil {
			if newIndex < 0 || newIndex >= len(AutobotColors) {
				result := inlineCommandInvalidArgument(
					cmd,
					"set_matcher_color",
					"color",
					color,
					fmt.Sprintf("color index must be between 0 and %d", len(AutobotColors)-1),
				)
				result.Result["matcher_name"] = cleanName
				result.Result["color"] = color
				return result
			}
			color = AutobotColors[newIndex]
		}
		// Pretty sure this is unused
		if color[:1] != "[" {
			color = "[" + color + "]"
		}
		reassignMatcherColor(cleanName, color)
		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "set_matcher_color")
		result.Result["matcher_name"] = cleanName
		result.Result["color"] = color
		return result

	case "fact":
		if len(tokens) < 2 {
			return inlineCommandRejected(cmd, "show_fact", "missing fact name")
		}
		args, _ := splitArgsShellStyle(commandLine[len(cmd):])
		f := args[0]
		if _, exists := factoidByName[strings.ToLower(f)]; !exists {
			pushErrorNow("Factoid not found: %s", f)
			result := inlineCommandRejected(cmd, "show_fact", "fact not found")
			result.Result["fact"] = f
			return result
		}
		pushFactNow(f, args[1:])
		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "show_fact")
		result.Result["fact"] = f
		return result
	case "alert", "ALERT":
		args, err := splitArgsShellStyle(commandLine[len(cmd):])
		if err != nil || len(args) < 1 {
			return inlineCommandRejected(cmd, "alert", "missing matcher name")
		}
		return invokeAlertCommand(cmd, args, line)
	case "alerts", "ALERTS":
		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "list_alerts")
		result.Result["active_alerts"] = activeAlertStates()
		result.Result["flux_depth"] = fluxDepth
		return result
	// === Live Actions (No args) ===
	case "demo", "DEMO":
		PattyGraph.demo = !PattyGraph.demo
		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "toggle_demo")
		result.Result["enabled"] = PattyGraph.demo
		return result
	case "facts.rnd", "FACTS.RND":
		doRandomFact = !doRandomFact
		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "toggle_random_facts")
		result.Result["enabled"] = doRandomFact
		return result
	case "ticker", "TICKER":
		PattyGraph.showTicker = !PattyGraph.showTicker
		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "toggle_ticker")
		result.Result["enabled"] = PattyGraph.showTicker
		return result
	case "history", "HISTORY":
		togglePreamble()
		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "toggle_fact_history")
		result.Result["enabled"] = showMetricsPanelContents
		return result
	case "expert", "EXPERT":
		expertMode = !expertMode
		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "toggle_expert")
		result.Result["enabled"] = expertMode
		return result
	case "control", "CONTROL":
		value := "on"
		if len(tokens) >= 2 {
			args, _ := splitArgsShellStyle(commandLine[len(cmd):])
			if len(args) > 0 {
				value = args[0]
			}
		}
		normalizedValue, ok := inlineBoolValue(value)
		if !ok {
			return inlineCommandInvalidArgument(cmd, "set_control_file_enabled", "control", value, "control requires a boolean value")
		}
		value = normalizedValue
		SetFlagByName("control", value)
		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "set_control_file_enabled")
		result.Result["value"] = enableControlFile
		return result
	// No-argument actions intentionally hard-stop after command recognition;
	// trailing text does not change or invalidate the requested action.
	case "purge", "PURGE":
		// TODO: can take optional matcher name
		purgePeakWordCommand()
		return inlineCommandResult(cmd, InlineCommandStatusApplied, "purge")
	case "pattySplat", "pattysplat", "PATTYSPLAT":
		pattySplat()
		return inlineCommandResult(cmd, InlineCommandStatusApplied, "write_splat")
	case "popBots", "popbots", "POPBOTS":
		PattyGraph.botsMatcher.migrateBots(-1)
		return inlineCommandResult(cmd, InlineCommandStatusApplied, "pop_bots")
	case "compact":
		compactCaches()
		return inlineCommandResult(cmd, InlineCommandStatusApplied, "compact_caches")
	case "dumpConfig", "dumpconfig", "DUMPCONFIG":
		dumpConfig()
		return inlineCommandResult(cmd, InlineCommandStatusApplied, "write_config")
	default:
		// Property settings consume one quote-aware value and intentionally stop;
		// each setting validates that value before SetFlagByName applies it.
		if len(tokens) < 2 {
			log.Printf("Missing value for property %s", cmd)
			return inlineCommandRejected(cmd, "set_flag", "missing value")
		}
		args, err := splitArgsShellStyle(commandLine[len(cmd):])
		if err != nil {
			return inlineCommandInvalidArgument(cmd, "set_flag", strings.ToLower(cmd), strings.TrimSpace(commandLine[len(cmd):]), err.Error())
		}
		if len(args) == 0 {
			return inlineCommandInvalidArgument(cmd, "set_flag", strings.ToLower(cmd), "", strings.ToLower(cmd)+" requires a value")
		}
		value := args[0]
		if message := validateInlineSettingArgument(cmd, value); message != "" {
			return inlineCommandInvalidArgument(cmd, "set_flag", strings.ToLower(cmd), value, message)
		}
		if strings.EqualFold(cmd, "save-dir") {
			expanded := expandUser(value)
			if opts.allowCreateSaveDir {
				if err := os.MkdirAll(expanded, 0o755); err != nil {
					log.Printf("Rejected save-dir %s: %v", expanded, err)
					result := inlineCommandRejected(cmd, "set_flag", fmt.Sprintf("save-dir unavailable: %v", err))
					result.Result["name"] = "save-dir"
					result.Result["value"] = value
					result.Result["path"] = expanded
					return result
				}
			} else if err := saveDirExists(expanded); err != nil {
				log.Printf("Rejected save-dir %s: %v", expanded, err)
				result := inlineCommandRejected(cmd, "set_flag", fmt.Sprintf("save-dir unavailable: %v", err))
				result.Result["name"] = "save-dir"
				result.Result["value"] = value
				result.Result["path"] = expanded
				return result
			}
		}
		if strings.EqualFold(cmd, "control-file") {
			PattyGraph.pattyConfig.setControlFile(value)
			if opts.allowCreateSaveDir {
				enableControlFile = true
			} else {
				pushSystemNow("Control file path saved for config")
			}
			result := inlineCommandResult(cmd, InlineCommandStatusApplied, "set_control_file_config")
			result.Result["name"] = "control-file"
			result.Result["value"] = value
			result.Result["path"] = PattyGraph.pattyConfig.controlFile
			if !opts.allowCreateSaveDir {
				result.Result["runtime_effect"] = "next_config_only"
			}
			return result
		}
		if strings.EqualFold(cmd, "json") || strings.EqualFold(cmd, "control") {
			normalizedValue, ok := inlineBoolValue(value)
			if !ok {
				return inlineCommandInvalidArgument(cmd, "set_flag", strings.ToLower(cmd), value, strings.ToLower(cmd)+" requires a boolean value")
			}
			value = normalizedValue
		}
		if !SetFlagByName(cmd, value) {
			log.Printf("Unknown inline command: %s", commandLine)
			return inlineCommandRejected(cmd, "unknown", "unknown inline command")
		}
		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "set_flag")
		result.Result["name"] = strings.ToLower(cmd)
		result.Result["value"] = value
		if strings.EqualFold(cmd, "json") {
			result.Result["enabled"] = generateSidecarJSONL
		}
		return result
	}
}

func invokeAlertCommand(cmd string, args []string, line string) InlineCommandResult {
	matcher := findMatcherByName(args[0])
	if matcher == nil {
		return inlineCommandRejected(cmd, "alert", "matcher not found")
	}
	if len(args) == 1 {
		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "show_alert")
		for key, value := range matcher.alertState() {
			result.Result[key] = value
		}
		return result
	}

	action := strings.ToLower(args[1])
	switch action {
	case AlertDirectionAbove, AlertDirectionBelow:
		if len(args) < 3 {
			return inlineCommandRejected(cmd, "set_alert", "missing threshold")
		}
		threshold, err := strconv.Atoi(args[2])
		if err != nil {
			return inlineCommandRejected(cmd, "set_alert", "invalid threshold")
		}
		if err := validateAlertBound(matcher, action, threshold); err != nil {
			return inlineCommandRejected(cmd, "set_alert", err.Error())
		}
		if action == AlertDirectionAbove {
			matcher.AlertAbove.set(threshold, line)
		} else {
			matcher.AlertBelow.set(threshold, line)
		}
		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "set_alert")
		result.Result["matcher"] = matcher.matcherName()
		result.Result["direction"] = action
		result.Result["threshold"] = threshold
		result.Result["flux_depth"] = fluxDepth
		return result
	case "clear":
		if len(args) > 2 {
			direction := strings.ToLower(args[2])
			if direction != AlertDirectionAbove && direction != AlertDirectionBelow {
				return inlineCommandRejected(cmd, "clear_alert", "unknown alert direction")
			}
			wasActive := clearMatcherAlertBound(matcher, direction)
			result := inlineCommandResult(cmd, InlineCommandStatusApplied, "clear_alert")
			result.Result["matcher"] = matcher.matcherName()
			result.Result["direction"] = direction
			result.Result["cleared"] = true
			result.Result["was_active"] = wasActive
			return result
		}
		aboveWasActive := matcher.AlertAbove.clear()
		belowWasActive := matcher.AlertBelow.clear()
		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "clear_alert")
		result.Result["matcher"] = matcher.matcherName()
		result.Result["cleared_above"] = true
		result.Result["above_was_active"] = aboveWasActive
		result.Result["cleared_below"] = true
		result.Result["below_was_active"] = belowWasActive
		return result
	default:
		return inlineCommandRejected(cmd, "alert", "unknown alert command")
	}
}

func findMatcherByName(name string) *Matcher {
	cleanName := strings.Trim(name, "*-+")
	for _, mf := range PattyGraph.matchers {
		matcher := mf.asMatcher()
		if matcher != nil && matcher.matcherName() == cleanName {
			return matcher
		}
	}
	return nil
}

func matcherNameExists(name string) bool {
	return findMatcherByName(name) != nil
}

func validateAlertBound(matcher *Matcher, direction string, threshold int) error {
	if threshold < 0 {
		return fmt.Errorf("alert threshold cannot be negative")
	}
	if direction == AlertDirectionBelow && threshold == 0 {
		return fmt.Errorf("below 0 is impossible for matcher counts")
	}
	switch direction {
	case AlertDirectionAbove:
		if matcher.AlertBelow.Enabled && matcher.AlertBelow.Threshold > threshold {
			return fmt.Errorf("below threshold must be <= above threshold")
		}
	case AlertDirectionBelow:
		if matcher.AlertAbove.Enabled && threshold > matcher.AlertAbove.Threshold {
			return fmt.Errorf("below threshold must be <= above threshold")
		}
	default:
		return fmt.Errorf("unknown alert direction")
	}
	return nil
}

func clearMatcherAlertBound(matcher *Matcher, direction string) bool {
	if direction == AlertDirectionAbove {
		return matcher.AlertAbove.clear()
	}
	return matcher.AlertBelow.clear()
}

func activeAlertStates() []map[string]interface{} {
	active := []map[string]interface{}{}
	for _, mf := range PattyGraph.matchers {
		matcher := mf.asMatcher()
		if matcher == nil {
			continue
		}
		active = append(active, matcher.activeAlertStates()...)
	}
	return active
}

func setMatcherMode(name string, newMode int) {
	var namedMatcher *Matcher
	cleanName := strings.Trim(name, "*-+")
	for _, mf := range PattyGraph.matchers {
		m := mf.asMatcher()
		if m != nil && m.name == cleanName {
			namedMatcher = m
			break
		}
	}
	if namedMatcher == nil {
		return
	}
	namedMatcher.displayMatchMode = newMode % 3
	namedMatcher.displayMatchedCache = ""
}

func colorInUse(color string) bool {
	for _, mf := range PattyGraph.matchers {
		m := mf.asMatcher()
		if m != nil && m.color == color {
			return true
		}
	}
	return false
}

func removeMatcher(name string) {
	removeMatcherFromTop(name, true)
}

func removeMatcherFromTop(name string, fromTop bool) {
	var matcher *Matcher

	if fromTop {
		for _, m := range PattyGraph.matchers {
			if m.matcherName() == name {
				matcher = m.asMatcher()
				break
			}
		}
	} else {
		for i := len(PattyGraph.matchers) - 1; i >= 0; i-- {
			m := PattyGraph.matchers[i]
			if m.matcherName() == name {
				matcher = m.asMatcher()
				break
			}
		}
	}
	if matcher == nil ||
		matcher == PattyGraph.bytesMatcher ||
		matcher == PattyGraph.linesMatcher ||
		matcher == PattyGraph.errsMatcher {
		return
	}
	if matcher == PattyGraph.botsMatcher {
		toggleBotsMatcher(true)
		return
	}
	newMatchers := make([]MatcherFacade, 0, len(PattyGraph.matchers)) // Preallocate
	for _, old := range PattyGraph.matchers {
		if old != matcher { // Keep all except selectedMatcher
			newMatchers = append(newMatchers, old)
		}
	}
	PattyGraph.matchers = newMatchers // Replace old slice
}

func toggleBotsMatcher(newState bool) {
	PattyGraph.botsMatcher.disableAutoAdd = newState
	if PattyGraph.botsMatcher.disableAutoAdd {
		PattyGraph.botsMatcher.setColor(PattyDisabledBotsColor)
	} else {
		PattyGraph.botsMatcher.setColor(PattyBotsColor)
	}
}
func purgePeakWordCommand() {
	PattyGraph.wordsMatcher.purgePeakWords()
	PattyGraph.refsMatcher.purgePeakWords()
	PattyGraph.ipsMatcher.purgePeakWords()
}
func pattySplat() {
	path, err := PattyGraph.printToFile()
	if err != nil {
		pushErrorNow("Splat write failed: %s", err)
		log.Printf("Splat write failed for %s: %v", path, err)
		return
	}
	pushSystemNow("Splat saved: %s", filepath.Base(path))
}

func dumpConfig() (string, error) {
	filename := newTimestampedFilename("pattyGraph_", ".conf")
	fullPath := filename
	if PattyGraph.pattyConfig.saveDir != "" {
		fullPath = filepath.Join(PattyGraph.pattyConfig.saveDir, filename)
	}
	f, err := os.Create(fullPath)
	if err != nil {
		pushErrorNow("Config write failed: %s", err)
		log.Printf("Config write failed for %s: %v", fullPath, err)
		return fullPath, err
	}
	defer f.Close()
	if err := writeConfig(f); err != nil {
		pushErrorNow("Config write failed: %s", err)
		log.Printf("Config write failed for %s: %v", fullPath, err)
		return fullPath, err
	}
	pushSystemNow("Config saved: %s", filepath.Base(fullPath))
	return fullPath, nil
}

func writeConfig(w io.Writer) error {
	write := func(format string, args ...any) error {
		_, err := fmt.Fprintf(w, format, args...)
		return err
	}
	// Config output is intentionally serialized as inline commands. Replay uses the
	// same interpreter as live control, which keeps saved matcher state and startup
	// configuration aligned with runtime behavior. This is why config generation
	// preserves original path expressions while runtime state uses resolved paths.
	if machineDisplayName != "" {
		if err := write(InlinePreamble+" title '%s'\n", machineDisplayName); err != nil {
			return err
		}
	}
	if PattyGraph.pattyConfig.saveDir != "" {
		if err := write(InlinePreamble+" save-dir '%s'\n", PattyGraph.pattyConfig.saveDirOriginal); err != nil {
			return err
		}
	}
	if PattyGraph.pattyConfig.jsonFileOriginal != "" {
		if err := write(InlinePreamble+" json-file '%s'\n", PattyGraph.pattyConfig.jsonFileOriginal); err != nil {
			return err
		}
	}
	if PattyGraph.pattyConfig.controlFileOriginal != "" {
		if err := write(InlinePreamble+" control-file '%s'\n", PattyGraph.pattyConfig.controlFileOriginal); err != nil {
			return err
		}
	}
	if pattyPushFactor != pattyPushFactorDefault {
		if err := write(InlinePreamble+" push %d\n", pattyPushFactor); err != nil {
			return err
		}
	}
	if pattyGracePeriod != pattyGracePeriodDefault {
		if err := write(InlinePreamble+" grace %d\n", pattyGracePeriod); err != nil {
			return err
		}
	}
	if fluxDepth != DefaultFluxDepth {
		if err := write(InlinePreamble+" flux %d\n", fluxDepth); err != nil {
			return err
		}
	}
	if pattyScaleFactor != pattyScaleFactorDefault {
		if err := write(InlinePreamble+" scale %1.1f\n", pattyScaleFactor); err != nil {
			return err
		}
	}
	if PattyGraph.pattyConfig.mbToRead != DefaultMBToRead {
		if err := write(InlinePreamble+" read %d\n", PattyGraph.pattyConfig.mbToRead); err != nil {
			return err
		}
	}
	if expertMode {
		if err := write(InlinePreamble + " expert\n"); err != nil {
			return err
		}
	}

	// Iterate through matchers and write their inline command representation
	for _, m := range PattyGraph.matchers {
		if m == nil {
			continue
		}
		cmd := m.asInlineCommand() // to be implemented per matcher
		if cmd != "" {
			if err := write("%s\n", cmd); err != nil {
				return err
			}
		}
		matcher := m.asMatcher()
		if matcher != nil && matcher.displayMatchMode != 0 {
			if err := write(InlinePreamble+" mode %s %d\n", matcher.name, matcher.displayMatchMode); err != nil {
				return err
			}
		}
		if matcher != nil {
			for _, alertLine := range matcher.alertConfigLines() {
				if err := write("%s\n", alertLine); err != nil {
					return err
				}
			}
		}
	}
	for _, m := range PattyGraph.matchers {
		if m == nil {
			continue
		}
		matcher := m.asMatcher()
		if matcher != nil && matcher.isColorUserAssigned {
			if err := write(InlinePreamble+" color %s %s\n", matcher.name, matcher.color); err != nil {
				return err
			}
		}
	}
	if err := write("#"+InlinePreamble+" color-index %d    # Next color index (Autogenerated)\n", colorIndex); err != nil {
		return err
	}
	return nil
}

func setFlux(f int) bool {
	if f < 1 || f > 10 {
		return false
	}
	if fluxDepth != f {
		fluxDepth = f
		resetAllAlertRuntimeState()
		return true
	}
	return false
}

// SetFlagByName is the shared setting mutator for inline commands and config
// replay. It is not raw pflag parsing: callers have already decided whether the
// source is trusted startup config or runtime input. Keep side effects here
// aligned with writeConfig and the public help text.
func SetFlagByName(key string, value string) bool {
	switch key {
	case "json":
		if value == "on" {
			generateSidecarJSONL = true
			sidecarWriteFailures = 0
			return true
		}
		if value == "off" {
			generateSidecarJSONL = false
			return true
		}
		return false
	case "control":
		enableControlFile = parseControlEnabled(value)
		return true
	case "control-file":
		PattyGraph.pattyConfig.setControlFile(value)
		return true
	case "push":
		if newPush, er := strconv.Atoi(value); er == nil {
			pushIncr := newPush - pattyPushFactor
			if PattyGraph.pattyPushFactorIncr(pushIncr) { // UI was already doing this so re-use until replaced
				pushFactNow("settings.push", nil)
			}
		}
		return true
	case "read":
		if newRead, er := strconv.Atoi(value); er == nil {
			PattyGraph.pattyConfig.mbToRead = newRead
		}
		return true
	case "grace":
		if newGrace, er := strconv.Atoi(value); er == nil {
			if setGracePeriod(newGrace) {
				pushFactNow("settings.grace", nil)
			}
		}
		return true
	case "flux":
		if f, er := strconv.Atoi(value); er == nil {
			if setFlux(f) {
				pushFactNow("settings.flux", nil)
			}
		}
		return true
	case "scale":
		if newScale, er := strconv.ParseFloat(value, 64); er == nil {
			// Truncate to one decimal place (flooring)
			setNewScaleFactor(newScale)
			pushFactNow("settings.scale", nil)
		}
		return true
	case "title":
		machineDisplayName = value
		return true
	case "save-dir":
		expanded := expandUser(value)
		if err := saveDirExists(expanded); err != nil {
			log.Printf("Rejected save-dir %s: %v", expanded, err)
			return true
		}
		PattyGraph.pattyConfig.setSaveDir(value)
		return true
	case "json-file":
		PattyGraph.pattyConfig.setJSONFile(value)
		generateSidecarJSONL = PattyGraph.pattyConfig.jsonFile != ""
		sidecarWriteFailures = 0
		return true
	case "color-index":
		if newIndex, er := strconv.Atoi(value); er == nil {
			if newIndex != colorIndex {
				colorIndex = newIndex // inline assignment
				pushFactNow("settings.color-index", nil)
			}
		}
		return true
	}
	return false
}

func setNewScaleFactor(originalScale float64) {
	// if you don't put your thumb on the scale, some 0.1 steps are skipped
	// 0.01 ensures things like 0.7999999 don't mess things up. Yes it happened.
	fudgedScale := originalScale + 0.01
	newScale := math.Floor(fudgedScale*10) / 10.0
	pattyScaleFactor = newScale
	if pattyScaleFactor > pattyBurstMax {
		pattyScaleFactor = pattyBurstMax
	}
	if pattyScaleFactor < pattyBurstMin {
		pattyScaleFactor = pattyBurstMin
	}
}

func setGracePeriod(newGrace int) bool {
	if newGrace < 2 {
		newGrace = 2
	}
	if newGrace > DefaultHistoryDepth {
		newGrace = DefaultHistoryDepth + 1
	}
	if newGrace != pattyGracePeriod {
		pattyGracePeriod = newGrace
		return true
	}
	return false
}
