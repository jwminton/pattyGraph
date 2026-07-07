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
	"strconv"
	"strings"
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

type InlineCommandOptions struct {
	allowCreateSaveDir bool
}

// invokeInlineCommand applies one runtime command after its source has already
// been accepted as command input.
func invokeInlineCommand(line string) InlineCommandResult {
	return invokeInlineCommandWithOptions(line, InlineCommandOptions{})
}

func invokeInlineCommandWithOptions(line string, opts InlineCommandOptions) InlineCommandResult {
	// Assume '!!! ' prefix already detected
	commandLine := line[4:]
	tokens := strings.Fields(commandLine)
	if len(tokens) == 0 {
		return inlineCommandResult("", InlineCommandStatusIgnored, "empty")
	}

	cmd := tokens[0]

	switch cmd {
	// === Matchers Management ===
	case "ADD", "add":
		// Parse command logLine with quote handling
		args, err := splitArgsShellStyle(commandLine[len(cmd):])
		if err != nil || len(args) < 1 {
			return inlineCommandRejected(cmd, "add_matcher", "missing matcher name")
		}

		isRegex := false
		name := args[0]
		scopeName := "line"
		patterns := []string{}
		// Check if second arg is a scope flag
		if len(args) > 1 {
			switch args[1] {
			case "--words":
				scopeName = "words"
			case "--refs":
				scopeName = "refs"
			case "--ips":
				scopeName = "ips"
			case "--code":
				scopeName = "code"
			case "--line":
				scopeName = "line"
			case "--regex":
				isRegex = true
			default:
				patterns = append(patterns, args[1])
			}
		}

		// Append any remaining args as patterns, trimming at '#' comment if present
		if len(args) > 2 {
			for _, arg := range args[2:] {
				if strings.HasPrefix(arg, "#") {
					break // Stop including args at the first '#' marker
				}
				patterns = append(patterns, arg)
			}
		}

		originalName := name
		if name[0:1] == "*" {
			name = name[1:]
		}
		if name[0:1] == "+" {
			name = name[1:]
		} else if name[0:1] == "-" {
			name = name[1:]
		}

		if len(patterns) == 0 {
			if name == "Browser" || name == "Platform" || name == "Bots" {
				isRegex = true
			} else {
				patterns = []string{name}
			}
		}
		if matcherNameExists(name) && !(name == "Bots" && isRegex && len(patterns) == 0) {
			return inlineCommandRejected(cmd, "add_matcher", "duplicate matcher name")
		}
		//newM := PattyGraph.createMatcher(name, isLikelyIPPattern(name), patterns)
		var newM *Matcher
		if isRegex {
			if name == "Browser" && len(patterns) == 0 {
				newM = newRegexMatcher(name, browserRegexString)
			} else if name == "Bots" {
				toggleBotsMatcher(false)
			} else if name == "Platform" && len(patterns) == 0 {
				newM = newRegexMatcher(name, platformRegexString)
			} else {
				index := strings.Index(commandLine, "--regex ")
				newM = newRegexMatcher(name, strings.TrimSpace(commandLine[index+8:]))
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
		// being nice for users doing a quick edit at the cli
		// add +Matcher ...
		// del +Matcher ....
		// they can just edit the action and leave the additional info on or off
		if name[0:1] == "*" {
			name = name[1:]
		}
		if name[0:1] == "+" {
			name = name[1:]
		} else if name[0:1] == "-" {
			name = name[1:]
			fromTop = false
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
		newMode, e := strconv.Atoi(args[1])
		if e != nil {
			return inlineCommandRejected(cmd, "set_matcher_mode", "invalid mode")
		}
		setMatcherMode(name, newMode)
		// being nice for users doing a quick edit at the cli
		// add +Matcher ...
		// del +Matcher ....
		// they can just edit the action and leave the additional info on or off
		if name[0:1] == "*" {
			name = name[1:]
		}
		if name[0:1] == "+" {
			name = name[1:]
		} else if name[0:1] == "-" {
			name = name[1:]
		}
		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "set_matcher_mode")
		result.Result["matcher_name"] = name
		result.Result["mode"] = newMode
		return result

	case "COLOR", "color":
		if len(tokens) < 3 {
			log.Printf("Invalid SET_COLOR usage: %s", commandLine)
			return inlineCommandRejected(cmd, "set_matcher_color", "missing matcher name or color")
		}
		name := tokens[1]
		color := tokens[2]
		if len(color) <= 2 {
			// color cannot be a valid color name or #FFFFFF value
			// must be an attempted index.
			if newIndex, err := strconv.Atoi(color); err == nil {
				if newIndex < len(AutobotColors) {
					color = AutobotColors[newIndex]
				}
			}
		}
		// Pretty sure this is unused
		if color[:1] != "[" {
			color = "[" + color + "]"
		}
		reassignMatcherColor(name, color)
		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "set_matcher_color")
		result.Result["matcher_name"] = name
		result.Result["color"] = color
		return result

	case "fact":
		if len(tokens) < 2 {
			return inlineCommandRejected(cmd, "show_fact", "missing fact name")
		}
		args, _ := splitArgsShellStyle(commandLine[len(cmd):])
		f := args[0]
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
		doRandom = !doRandom
		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "toggle_random_facts")
		result.Result["enabled"] = doRandom
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
		SetFlagByName("control", value)
		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "set_control_file_enabled")
		result.Result["value"] = enableControlFile
		return result
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
		// === Property Settings (single arg) ===
		if len(tokens) < 2 {
			log.Printf("Missing value for property %s", cmd)
			return inlineCommandRejected(cmd, "set_flag", "missing value")
		}
		args, _ := splitArgsShellStyle(commandLine[len(cmd):])
		value := args[0]
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
		if !SetFlagByName(cmd, value) {
			log.Printf("Unknown inline command: %s", commandLine)
			return inlineCommandRejected(cmd, "unknown", "unknown inline command")
		}
		result := inlineCommandResult(cmd, InlineCommandStatusApplied, "set_flag")
		result.Result["name"] = strings.ToLower(cmd)
		result.Result["value"] = value
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
		} else {
			generateSidecarJSONL = PattyGraph.pattyConfig.jsonFile != ""
		}
		return true
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
