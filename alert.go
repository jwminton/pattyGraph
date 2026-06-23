// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import "strconv"

const (
	AlertDirectionAbove = "above"
	AlertDirectionBelow = "below"

	AlertStatusTriggered = "triggered"
	AlertStatusRecovered = "recovered"
)

type AlertBound struct {
	Enabled   bool
	Threshold int
	Active    bool
	HitRun    int
	ClearRun  int
	LastValue int
}

type AlertTransition struct {
	Status       string
	MatcherName  string
	Direction    string
	Value        int
	Threshold    int
	FluxDepth    int
	Streak       int
	Interval     int
	CurrentCycle int
}

func (b *AlertBound) set(threshold int) {
	*b = AlertBound{
		Enabled:   true,
		Threshold: threshold,
	}
}

func (b *AlertBound) clear() bool {
	wasActive := b.Active
	*b = AlertBound{}
	return wasActive
}

func (b *AlertBound) resetRuntimeState() {
	b.Active = false
	b.HitRun = 0
	b.ClearRun = 0
	b.LastValue = 0
}

func resetAllAlertRuntimeState() {
	if PattyGraph == nil {
		return
	}
	for _, mf := range PattyGraph.matchers {
		matcher := mf.asMatcher()
		if matcher == nil {
			continue
		}
		matcher.AlertAbove.resetRuntimeState()
		matcher.AlertBelow.resetRuntimeState()
	}
	PattyGraph.clearPendingAlertTransitions()
}

func (m *Matcher) evaluateAlertBounds() {
	value := m.intervalCount
	m.evaluateAlertBound(AlertDirectionAbove, &m.AlertAbove, value)
	m.evaluateAlertBound(AlertDirectionBelow, &m.AlertBelow, value)
}

func (m *Matcher) evaluateAlertBound(direction string, bound *AlertBound, value int) {
	if bound == nil || !bound.Enabled {
		return
	}

	bound.LastValue = value
	hit := alertBoundHit(direction, value, bound.Threshold)
	if hit {
		bound.HitRun++
		bound.ClearRun = 0
	} else {
		bound.ClearRun++
		bound.HitRun = 0
	}

	if !bound.Active && bound.HitRun >= fluxDepth {
		bound.Active = true
		m.registerAlertTransition(AlertStatusTriggered, direction, bound, value, bound.HitRun)
		return
	}
	if bound.Active && bound.ClearRun >= fluxDepth {
		bound.Active = false
		m.registerAlertTransition(AlertStatusRecovered, direction, bound, value, bound.ClearRun)
	}
}

func alertBoundHit(direction string, value int, threshold int) bool {
	switch direction {
	case AlertDirectionAbove:
		return value >= threshold
	case AlertDirectionBelow:
		return value < threshold
	default:
		return false
	}
}

func (m *Matcher) registerAlertTransition(status string, direction string, bound *AlertBound, value int, streak int) {
	if PattyGraph == nil {
		return
	}
	PattyGraph.registerAlertTransition(AlertTransition{
		Status:       status,
		MatcherName:  m.matcherName(),
		Direction:    direction,
		Value:        value,
		Threshold:    bound.Threshold,
		FluxDepth:    fluxDepth,
		Streak:       streak,
		Interval:     PattyGraph.intervalsCompleted,
		CurrentCycle: currentCycle,
	})
}

func (m *Matcher) alertBoundState(direction string, bound AlertBound) map[string]interface{} {
	out := map[string]interface{}{
		"direction": direction,
		"enabled":   bound.Enabled,
	}
	if !bound.Enabled {
		return out
	}
	out["threshold"] = bound.Threshold
	out["active"] = bound.Active
	out["hit_run"] = bound.HitRun
	out["clear_run"] = bound.ClearRun
	out["last_value"] = bound.LastValue
	return out
}

func (m *Matcher) alertState() map[string]interface{} {
	return map[string]interface{}{
		"matcher":    m.matcherName(),
		"flux_depth": fluxDepth,
		"above":      m.alertBoundState(AlertDirectionAbove, m.AlertAbove),
		"below":      m.alertBoundState(AlertDirectionBelow, m.AlertBelow),
	}
}

func (m *Matcher) activeAlertStates() []map[string]interface{} {
	active := []map[string]interface{}{}
	if m.AlertAbove.Enabled && m.AlertAbove.Active {
		active = append(active, m.activeAlertState(AlertDirectionAbove, m.AlertAbove))
	}
	if m.AlertBelow.Enabled && m.AlertBelow.Active {
		active = append(active, m.activeAlertState(AlertDirectionBelow, m.AlertBelow))
	}
	return active
}

func (m *Matcher) activeAlertState(direction string, bound AlertBound) map[string]interface{} {
	return map[string]interface{}{
		"matcher":    m.matcherName(),
		"direction":  direction,
		"value":      bound.LastValue,
		"threshold":  bound.Threshold,
		"streak":     bound.HitRun,
		"flux_depth": fluxDepth,
	}
}

func (m *Matcher) alertConfigLines() []string {
	lines := []string{}
	matcherName := quoteInlineArg(m.matcherName())
	if m.AlertBelow.Enabled {
		lines = append(lines, InlinePreamble+" alert "+matcherName+" below "+strconv.Itoa(m.AlertBelow.Threshold))
	}
	if m.AlertAbove.Enabled {
		lines = append(lines, InlinePreamble+" alert "+matcherName+" above "+strconv.Itoa(m.AlertAbove.Threshold))
	}
	return lines
}

func quoteInlineArg(arg string) string {
	if arg == "" {
		return "''"
	}
	needsQuote := false
	for _, r := range arg {
		if r == ' ' || r == '\t' || r == '\'' || r == '"' || r == '#' {
			needsQuote = true
			break
		}
	}
	if !needsQuote {
		return arg
	}
	out := "'"
	for _, r := range arg {
		if r == '\'' {
			out += "'\"'\"'"
		} else {
			out += string(r)
		}
	}
	out += "'"
	return out
}
