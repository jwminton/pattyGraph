// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"fmt"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"log"
	"strings"
	"time"
)

func tabGlyph() string {
	symbols := []string{"-", "/", "|", "\\", "=", "_"}
	return symbols[PattyGraph.tabViewIndexKey%len(symbols)]
}
func columnRuler() string {
	return fillRuler(PattyPrintWidth, true)
}

func fillRuler(width int, multiLine bool) string {
	var digits1, digits10 string
	for i := 1; i <= width; i++ {
		digits1 += fmt.Sprintf("%d", i%10)
		if i%10 == 0 {
			digits10 += fmt.Sprintf("%d", (i/10)%10)
		} else {
			digits10 += " "
		}
	}
	if multiLine {
		return digits10 + "\n" + digits1
	}
	return digits1
}

// Expensive but cached
func timeScaleWithHighlights() string {
	if timeScaleCache == "" {
		if expertMode {
			timeScaleCache = createTimeScaleWithHighlights(ExpertScaleWidth)
		} else {
			timeScaleCache = createTimeScaleWithHighlights(DefaultHistoryDepth)
		}
	}
	return timeScaleCache
}

var timeScaleCache string

// Expensive but cached
func createTimeScaleWithHighlights(displayWidth int) string {
	const highlight = "[white]"
	const reset = "[default]"
	var b strings.Builder
	pos := 0
	b.WriteString(reset)
	for pos <= displayWidth {
		if pos == 0 {
			if pos == PattyGraph.miniWindowIndex {
				b.WriteString(fmt.Sprintf("%s%s%s", highlight, "/60", reset))
			} else {
				b.WriteString("/60")
			}
			pos += 3
		} else if pos >= 80 {
			if pos == PattyGraph.miniWindowIndex {
				b.WriteString(fmt.Sprintf("%s%s%s", highlight, "8", reset))
			} else {
				b.WriteString("8")
			}
			pos += 1
		} else if pos == 5 || pos%10 == 0 {
			if pos == PattyGraph.miniWindowIndex {
				b.WriteString(fmt.Sprintf("%s%2d%s", highlight, pos, reset))
			} else {
				b.WriteString(fmt.Sprintf("%2d", pos))
			}
			pos += 2
		} else if pos > 10 && pos%5 == 0 {
			if pos == PattyGraph.miniWindowIndex {
				b.WriteString(fmt.Sprintf("%s%s%s", highlight, ".", reset))
			} else {
				b.WriteString(".")
			}
			pos += 1
		} else {
			b.WriteByte(' ')
			pos += 1
		}
	}
	return b.String()
}

type BuilderComplex struct {
	graphingBuilder strings.Builder
	matcherBuilder  strings.Builder
}

// Reset clears all builders
func (b *BuilderComplex) Reset() {
	b.graphingBuilder.Reset()
	b.matcherBuilder.Reset()
}

// Builders reused during display to avoid repeated allocation in the refresh path.
var PattyGraphBuilderComplex BuilderComplex

func updateDisplay() {
	defer PattyGraphBuilderComplex.Reset()
	// Build main display text

	var progressPanel string
	// For debugging only. Throws off click detection bc 2 lines not accounted for
	//PattyGraphBuilderComplex.graphingBuilder.WriteString(columnRuler() + "\n") // debugging ruler lines for alignment w/o manual counting

	if PattyGraph.totalLines < 10000000 {
		progressPanel = fmt.Sprintf("%7d/%d/%-4s", PattyGraph.totalLines, PattyGraph.intervalsCompleted, strings.TrimSpace(formatBytes64(PattyGraph.totalBytes)))
	} else {
		progressPanel = fmt.Sprintf("%7s/%d/%-4s", formatCountsUint64(PattyGraph.totalLines), PattyGraph.intervalsCompleted, strings.TrimSpace(formatBytes64(PattyGraph.totalBytes)))
	}

	selectionValue := "-"
	if PattyGraph.selectedGraphValue > 0 {
		selectionValue = PattyGraph.selectionValue
	}

	PattyGraphBuilderComplex.graphingBuilder.WriteString(fmt.Sprintf("[white]File: %-30s%18.18s%20.20s %-4s %20s\n",
		PattyGraph.filePath,
		machineDisplayName,
		progressPanel,
		selectionValue,
		PattyGraph.logtime.Format("Jan 02,2006 15:04:05")))
	timeScaleLine := timeScaleWithHighlights()

	progressColor := "yellow"
	if displayFreezeCountdown > 0 {
		progressColor = "red"
	}

	if expertMode {
		internalState := PattyGraph.statusLine()
		//internalState = fillRuler(37, false)
		PattyGraphBuilderComplex.graphingBuilder.WriteString(fmt.Sprintf(" ["+progressColor+"]%15.15s[white]%3d%-44s[white]%37s\n",
			renderProgressBar(), currentCycle, timeScaleLine, internalState))
	} else {
		PattyGraphBuilderComplex.graphingBuilder.WriteString(fmt.Sprintf(" ["+progressColor+"]%15.15s[white]%3d%80s\n",
			renderProgressBar(), currentCycle, timeScaleLine))

	}

	// Recompute global history-wide scale lazily after push invalidates it.
	if PattyGraph.overallMax <= 0 {
		// These values only change after a push(), so this tries to cache them between pushes
		// push sets overallMax to -1 to signal a recomputation be done here
		PattyGraph.overallMin, _, PattyGraph.overallMax = PattyGraph.minAvgMaxHistoryAcrossMatchers()
	}

	switch bottomPanelCurrent {
	case bottomPanelHelp:
		PattyGraphBuilderComplex.matcherBuilder.WriteString(quickHelpPanelContents())
	case bottomPanelFactoids:
		PattyGraphBuilderComplex.matcherBuilder.WriteString(metricPanelContents())
	default:
		for _, matcherFacade := range PattyGraph.matchers {
			matcher := matcherFacade.asMatcher()
			if matcher != nil {
				PattyGraphBuilderComplex.matcherBuilder.WriteString(matcher.displayMatched())
			}
		}
	}
	var wordsPanel, refsPanel, ipsPanel string
	if displayFreezeCountdown == 0 {
		// panel 2
		wordsPanel = PattyGraph.wordsMatcher.displayMatched()
		// panel 3
		refsPanel = PattyGraph.refsMatcher.displayMatched()
		// panel 4
		ipsPanel = PattyGraph.ipsMatcher.displayMatched()
	}

	// IMPORTANT: This must be done after word matcher matching so spoofed selection values are created
	// Draws matcher labeling and sparkline
	for _, matcher := range PattyGraph.matchers {
		PattyGraphBuilderComplex.graphingBuilder.WriteString(matcher.displayString())
	}
	if PattyGraph.showTicker {
		PattyGraphBuilderComplex.graphingBuilder.WriteString("[default:-]")
		PattyGraphBuilderComplex.graphingBuilder.WriteString(currentTickerText())
		PattyGraphBuilderComplex.graphingBuilder.WriteString("[-:-]\n")
	}

	if PattyGraph.selectedInterestingMatcher != nil {
		PattyGraphBuilderComplex.graphingBuilder.WriteString(PattyGraph.selectedInterestingMatcher.displayLogLine())
	}
	if PattyGraph.selectedMatcher != nil {
		PattyGraphBuilderComplex.graphingBuilder.WriteString(PattyGraph.selectedMatcher.displayLogLine())
	}

	PattyGraph.sparklineHistoryView.SetText(PattyGraphBuilderComplex.graphingBuilder.String())
	PattyGraph.botMatchesView.SetText(PattyGraphBuilderComplex.matcherBuilder.String())
	if displayFreezeCountdown == 0 {
		PattyGraph.wordMatchesView.SetText(wordsPanel)
		PattyGraph.refsView.SetText(refsPanel)
		PattyGraph.ipsView.SetText(ipsPanel)
	}
	relayoutUI()
}

func (m *Monitor) pattyPushFactorIncr(increment int) bool {
	prePush := pattyPushFactor
	pattyPushFactor += increment
	if pattyPushFactor < 0 {
		pattyPushFactor = 0
	} else if pattyPushFactor > 11 {
		pattyPushFactor = 11
	}
	if prePush != pattyPushFactor {
		wordsPurgeInterval, refsPurgeInterval, ipsPurgeInterval := lookupPurgeIntervals(pattyPushFactor)
		m.wordsMatcher.setPurgeInterval(wordsPurgeInterval)
		m.refsMatcher.setPurgeInterval(refsPurgeInterval)
		m.ipsMatcher.setPurgeInterval(ipsPurgeInterval)
		return true
	}
	return false
}

/*
*

	PattyGraph intentionally treats the terminal screen as a rendered coordinate
	surface rather than a tree of tview widgets. The display panes are text views,
	but the interaction model is visual: clicks are interpreted by X/Y position
	against stable screen regions such as the matcher graph, matcher breakdowns,
	words, refs, and IPs.

	setUIHook is therefore the TUI controller. It maps keyboard and mouse gestures
	onto PattyGraph’s live operational model: matcher selection, interesting-key
	selection, graph value inspection, display-mode cycling, matcher promotion, and
	runtime tuning.

	This function is the spatial controller for the TUI: tview widgets provide
	rendering surfaces, while PattyGraph owns the section-based gesture vocabulary.
*/
func setUIHook() {
	PattyGraph.app.SetMouseCapture(func(event *tcell.EventMouse, action tview.MouseAction) (*tcell.EventMouse, tview.MouseAction) {
		mLen := len(PattyGraph.matchers)
		// Hit detection is tied to the stable text layout rendered above: matcher
		// rows first, then optional ticker/selection rows, then interesting lists.
		if action == tview.MouseLeftClick {
			sHeight := PattyGraph.sparkPanelHeight()
			hasControlKey := event.Modifiers()&tcell.ModCtrl != 0
			x, y := event.Position()
			//PattyGraph.selectedGraphPosition = fmt.Sprintf("%d:%d", x, y)
			// spark graph click
			offset, _ := PattyGraph.sparklineHistoryView.GetScrollOffset()
			if x > PattyPrintWidth || y < (2-offset) {
				return event, action
			}
			// Matcher rows occupy the spark panel before the fixed interesting
			// streams and selected-interesting row.
			if y < sHeight {
				index := y - 2
				matcherCount := mLen - 3
				index += offset
				// matcher & ticker selection management
				if index < matcherCount {
					newM := PattyGraph.matchers[index].asMatcher()
					if PattyGraph.selectedMatcher == newM && x < 20 {
						if hasControlKey {
							//newM.displayMatchZero = !newM.displayMatchZero
							newM.displayMatchMode = (newM.displayMatchMode + 1) % 3
							newM.displayMatchedCache = ""
						} else {
							PattyGraph.setSelectedMatcher(nil)
						}
					} else {
						PattyGraph.setSelectedMatcher(newM)
						if hasControlKey {
							//newM.displayMatchZero = !newM.displayMatchZero
							newM.displayMatchMode = (newM.displayMatchMode + 1) % 3
							newM.displayMatchedCache = ""
						}
					}
				} else if PattyGraph.showTicker &&
					((PattyGraph.selectedInterestingMatcher != nil && index == matcherCount+1) ||
						(PattyGraph.selectedInterestingMatcher == nil && index == matcherCount)) {
					togglePreamble()
				}

				//// selected interesting sparkline value selection
				if y == mLen-1 && PattyGraph.selectedInterestingMatcher != nil {
					//m.setSelectedMatcher(nil)
					if x >= 20 && x < PattyPrintWidth {
						idx := x - 20
						PattyGraph.selectedGraphValue = PattyGraph.selectedInterestingMatcher.selectedHistoryAt(idx)
						PattyGraph.selectionValue = fmt.Sprintf("%d", PattyGraph.selectedGraphValue)
						if PattyGraph.showTicker {
							pushPrintStatCycles(
								PattyGraph.selectedInterestingMatcher.selectedKey,
								PattyGraph.selectedGraphValue,
								idx,
							)
						}
					}
				} else if y <= mLen-1 && PattyGraph.selectedMatcher != nil && x >= 20 && x < PattyPrintWidth {
					// Getting the idx into the spoofed wordStats
					idx := x - 19
					if PattyGraph.selectedMatcher.history != nil && idx <= len(PattyGraph.selectedMatcher.history) {
						PattyGraph.selectedGraphValue = PattyGraph.selectedMatcher.history[len(PattyGraph.selectedMatcher.history)-idx]
						if PattyGraph.selectedMatcher == PattyGraph.bytesMatcher {
							PattyGraph.selectionValue = fmt.Sprintf("%s", formatBytes(PattyGraph.selectedGraphValue))
						} else if PattyGraph.selectedMatcher == PattyGraph.linesMatcher {
							PattyGraph.selectionValue = fmt.Sprintf("%s", formatCounts(PattyGraph.selectedGraphValue))
						} else {
							if PattyGraph.selectedGraphValue > 10000 {
								PattyGraph.selectionValue = trimmedCounts(PattyGraph.selectedGraphValue)
							} else {
								PattyGraph.selectionValue = fmt.Sprintf("%d", PattyGraph.selectedGraphValue)
							}
						}
						if PattyGraph.showTicker {
							pushPrintStatCycles(
								PattyGraph.selectedMatcher.matcherName(),
								PattyGraph.selectionValue,
								idx,
							)
						}
					} else {
						// out of bounds catchall
						PattyGraph.selectedGraphValue = -1
						//pushPrintNow("bounds checker")
					}
				}
				return nil, action
				//} else if  {
				//
			} else {
				// below the spark graph. One of the four columns should take "focus" and translate the click to a
				// selection. Interesting columns only look for/print the selectionKey if they have focus
				if x <= botsDisplayWidth {
					if bottomPanelCurrent == bottomPanelMatchers {
						// MATCHERS Column
						offset, _ := PattyGraph.botMatchesView.GetScrollOffset()
						index := y - PattyGraph.sparkPanelHeight() + offset
						slide := 0 // simulated indexing to make it "chunkier"  (i.e. one matcher can == multiple lines)
						for i, facade := range PattyGraph.matchers {
							matcher := facade.asMatcher()
							if matcher != nil {
								//if matcher.name != "bytes" {
								//if matcher.name != "lines" && matcher.name != "bytes" {
								if index >= i+slide && index <= i+slide+matcher.matchedDisplayCount {
									if PattyGraph.selectedMatcher == matcher {
										if hasControlKey {
											//matcher.displayMatchZero = !matcher.displayMatchZero
											matcher.displayMatchMode = (matcher.displayMatchMode + 1) % 3
											matcher.displayMatchedCache = ""
										} else {
											PattyGraph.setSelectedMatcher(nil)
										}
									} else {
										PattyGraph.setSelectedMatcher(matcher)
										if hasControlKey {
											//matcher.displayMatchZero = !matcher.displayMatchZero
											matcher.displayMatchMode = (matcher.displayMatchMode + 1) % 3
											matcher.displayMatchedCache = ""
										}
									}
									break
								}
								//autoMatcher.toggleSelection(index >= i+slide && index <= i+slide+autoMatcher.matchedDisplayCount)
								slide += matcher.matchedDisplayCount
								//} else {
								//	// lines and bytes get skipped and have to reset the sliding index
								//	//autoMatcher.toggleSelection(false)
								//	slide--
								//}
							}
						}
					}
				} else if x < botsDisplayWidth+PattyGraph.wordsMatcher.displayWidth {
					// words
					offset, _ := PattyGraph.wordMatchesView.GetScrollOffset()
					index := y - PattyGraph.sparkPanelHeight() + offset - 1
					//m.setSelectedMatcher(nil)
					PattyGraph.wordsMatcher.selectDisplayItem(index)
				} else if x < botsDisplayWidth+PattyGraph.wordsMatcher.displayWidth+PattyGraph.refsMatcher.displayWidth {
					// refs
					offset, _ := PattyGraph.refsView.GetScrollOffset()
					index := y - PattyGraph.sparkPanelHeight() + offset - 1
					//m.setSelectedMatcher(nil)
					PattyGraph.refsMatcher.selectDisplayItem(index)
				} else if x < botsDisplayWidth+PattyGraph.wordsMatcher.displayWidth+PattyGraph.refsMatcher.displayWidth+PattyGraph.ipsMatcher.displayWidth {
					// ips
					offset, _ := PattyGraph.ipsView.GetScrollOffset()
					index := y - PattyGraph.sparkPanelHeight() + offset - 1
					//m.setSelectedMatcher(nil)
					PattyGraph.ipsMatcher.selectDisplayItem(index)
				}
			}

		}
		return event, action
	})

	PattyGraph.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// key input dispatch logic
		switch event.Key() {
		case tcell.KeyRune:
			//if event.Modifiers()&tcell.ModCtrl != 0 {
			switch event.Rune() {
			case '<': // closest to Ctrl-<
				setGracePeriod(pattyGracePeriod - 1)
				return nil
			case '>': // closest to Ctrl->
				setGracePeriod(pattyGracePeriod + 1)
				return nil
			case '{':
				setFlux(fluxDepth - 1)
				return nil
			case '}':
				setFlux(fluxDepth + 1)
				return nil
			case '[':
				PattyGraph.miniWindowIndex -= 5
				if PattyGraph.miniWindowIndex < 0 {
					PattyGraph.miniWindowIndex = 0
				}
				timeScaleCache = ""
				return nil
			case ']':
				PattyGraph.miniWindowIndex += 5
				if PattyGraph.miniWindowIndex > 75 {
					PattyGraph.miniWindowIndex = 75
				}
				timeScaleCache = ""
				return nil
			case 'x':
				expertMode = !expertMode
				timeScaleCache = ""
				return nil
			case 'X':
				PattyGraph.showTicker = !PattyGraph.showTicker
				timeScaleCache = ""
				return nil
			case 'f':
				// 5+1 bc current cycle
				displayFreezeCountdown = 6
				return nil
			case 'q':
				PattyGraph.app.Stop()
				return nil
			case 'F':
				// 10+1 bc current cycle
				displayFreezeCountdown = 11
				return nil
			case 'U':
				if PattyGraph.selectedMatcher == nil ||
					PattyGraph.selectedMatcher.name == "lines" ||
					PattyGraph.selectedMatcher.name == "bytes" ||
					PattyGraph.selectedMatcher.name == "errs" {
					return nil
				}

				for i := 1; i < len(PattyGraph.matchers); i++ {
					if PattyGraph.matchers[i].asMatcher() == PattyGraph.selectedMatcher {
						// Swap with the one above it
						PattyGraph.matchers[i], PattyGraph.matchers[i-1] = PattyGraph.matchers[i-1], PattyGraph.matchers[i]
						break
					}
				}
				UpdateHistoricFlags()
				return nil
			case 'D':
				if PattyGraph.selectedMatcher == nil ||
					PattyGraph.selectedMatcher.name == "lines" ||
					PattyGraph.selectedMatcher.name == "bytes" ||
					PattyGraph.selectedMatcher.name == "errs" {
					return nil
				}
				for i := 0; i < len(PattyGraph.matchers)-1; i++ {
					if PattyGraph.matchers[i+1].asMatcher() == PattyGraph.linesMatcher {
						break
					}
					if PattyGraph.matchers[i].asMatcher() == PattyGraph.selectedMatcher {
						// Swap with the one after it
						PattyGraph.matchers[i], PattyGraph.matchers[i+1] = PattyGraph.matchers[i+1], PattyGraph.matchers[i]
						break
					}
				}
				UpdateHistoricFlags()
				return nil
			}

		case tcell.KeyTab:
			if PattyGraph.demo {
				PattyGraph.demo = false
			} else {
				PattyGraph.tabViewIndexKey = (PattyGraph.tabViewIndexKey + 1) % SecondaryInfoTabDepth
			}
			return nil
		case tcell.KeyRight:
			if event.Modifiers()&tcell.ModCtrl != 0 {
				PattyGraph.pattyPushFactorIncr(1)
				return nil
			}
		case tcell.KeyLeft:
			if event.Modifiers()&tcell.ModCtrl != 0 {
				PattyGraph.pattyPushFactorIncr(-1)
				return nil
			}
		case tcell.KeyUp:
			if event.Modifiers()&tcell.ModCtrl != 0 {
				setNewScaleFactor(pattyScaleFactor + pattyBurstStep)
				return nil
			}
		case tcell.KeyDown:
			if event.Modifiers()&tcell.ModCtrl != 0 {
				setNewScaleFactor(pattyScaleFactor - pattyBurstStep)
				return nil
			}
		case tcell.KeyCtrlF:
			doRandom = !doRandom
			//firstColorWins = !firstColorWins
			return nil
		case tcell.KeyCtrlH:
			toggleHelpPanel()
			return nil
		case tcell.KeyCtrlP:
			// Purge! Clear all PeakWords!
			PattyGraph.purgeAllPeakContent()
			return nil
		case tcell.KeyCtrlD:
			if PattyGraph.selectedMatcher == PattyGraph.botsMatcher {
				toggleBotsMatcher(!PattyGraph.botsMatcher.disableAutoAdd)
			} else {
				// Delete Selected AutoMatcher (if there is one)
				PattyGraph.deleteSelectedMatcher()
			}
			return nil
		case tcell.KeyEscape:
			if PattyGraph.selectedMatcher != nil {
				PattyGraph.setSelectedMatcher(nil)
			} else if PattyGraph.selectedInterestingMatcher != nil {
				PattyGraph.selectedInterestingMatcher.selectedKey = ""
				PattyGraph.selectedInterestingMatcher.selectedGraphCache = ""
				PattyGraph.selectedInterestingMatcher = nil
			} else {
				PattyGraph.selectedGraphValue = 0
			}
			return nil
		case tcell.KeyCtrlG:
			dumpConfig()
			return nil
		case tcell.KeyCtrlS:
			pattySplat()
			return nil
		case tcell.KeyCtrlM, tcell.KeyCtrlB, tcell.KeyCtrlN:
			if event.Key() == tcell.KeyCtrlM && PattyGraph.selectedMatcher == PattyGraph.botsMatcher {
				PattyGraph.botsMatcher.migrateBots(-1)
			} else if PattyGraph.selectedInterestingMatcher != nil {
				newPattern := PattyGraph.selectedInterestingMatcher.selectedKey
				if newPattern != "" {
					if matcherNameExists(newPattern) {
						return nil
					}
					var newM *Matcher
					fmtString := ""
					if event.Key() == tcell.KeyCtrlN {
						fmtString = "*"
					}

					switch PattyGraph.selectedInterestingMatcher {
					case PattyGraph.refsMatcher:
						newM = RefsMatcher(newPattern, []string{newPattern})
						newM.inlineCommandAction = func() string {
							return fmt.Sprintf(InlinePreamble+" add %s%s --refs", fmtString, newPattern)
						}
					case PattyGraph.ipsMatcher:
						adjusted := newPattern
						if strings.Count(newPattern, ".") == 1 {
							adjusted = newPattern + "."
						}
						newM = IpsMatcher(newPattern, []string{adjusted})
						newM.inlineCommandAction = func() string {
							return fmt.Sprintf(InlinePreamble+" add %s%s --ips", fmtString, adjusted)
						}
					case PattyGraph.wordsMatcher:
						newM = WordsMatcher(newPattern, []string{newPattern})
						newM.inlineCommandAction = func() string {
							return fmt.Sprintf(InlinePreamble+" add %s%s --words", fmtString, newPattern)
						}
					}
					if entry, exists := PattyGraph.selectedInterestingMatcher.wordFrequency[newPattern]; exists {
						newM.intervalCount = entry.count
						newM.history = entry.historySlice() // matcher creation
					} else if PattyGraph.selectedInterestingMatcher == PattyGraph.ipsMatcher && PattyGraph.ipsMatcher.ipScratch != nil {
						// Prefix-group selections come from displayIpGroups; use the
						// scratch aggregate as the source for the promoted matcher.
						if newHistory, exists2 := PattyGraph.ipsMatcher.ipScratch.prefixHistorAggregateBufs[newPattern]; exists2 {
							newM.intervalCount = PattyGraph.ipsMatcher.ipScratch.prefixCounts[newPattern]
							newM.history = newHistory.Slice()
						}
					}

					if event.Key() == tcell.KeyCtrlB {
						PattyGraph.matchers = insertMatcherBeforeBots(PattyGraph.matchers, newM)
					} else if event.Key() == tcell.KeyCtrlN {
						PattyGraph.matchers = insertMatcherBeforeLines(PattyGraph.matchers, newM)
					} else {
						PattyGraph.matchers = insertMatcherFirst(PattyGraph.matchers, newM)
					}
				}
			}
			return nil
		}
		return event // Pass other events through
	})
}

func (m *Monitor) setSelectedMatcher(matcher *Matcher) {
	if m.selectedMatcher != nil {
		m.selectedMatcher.displayMatchedCache = ""
	}
	if matcher != nil {
		matcher.displayMatchedCache = ""
		if m.showTicker {
			matcher.pushStats()
		}
	}
	m.selectedMatcher = matcher // only mutator
}
func (m *Monitor) createMatcher(newPattern string, startsWith bool, patterns []string) *Matcher {
	var newM *Matcher
	if startsWith {
		// An IP matcher whether from config, inline or gesture
		//newM = StartsWithMatcher(newPattern, patterns)
		newM = IpsMatcher(newPattern, patterns)
	} else {
		newM = SimplePredicateMatcher(newPattern, patterns)
	}
	//var existingMatcher *InterestingWordMatcher
	for _, wordMatcher := range []*InterestingWordMatcher{m.ipsMatcher, m.refsMatcher, m.wordsMatcher} {
		if entry, exists := wordMatcher.wordFrequency[newPattern]; exists {
			newM.intervalCount = entry.count
			newM.history = entry.historySlice() // matcher creation
			//newHistory := make([]int, entry.historyLength())
			//copy(newHistory, entry.historySlice()) // gets reversed
			//newM.history = reversedCopy(newHistory)
			//existingMatcher = wordMatcher
			break
		}
	}
	// might need to be a part of actually adding?
	//if existingMatcher != nil && existingMatcher.wordFrequency[newPattern] != nil {
	//	existingMatcher.wordFrequency[newPattern].spawned = true
	//}
	return newM
}

var startTime time.Time

// Starts the UI and refreshes it every second to display the latest logLine intervalCount
func startUI() {
	// force the first relayout
	layoutUI()
	setUIHook()
	// Yes, here again on purpose
	initCycle()
	app := PattyGraph.app
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		startTime = time.Now()
		defer ticker.Stop()
		for range ticker.C {
			app.QueueUpdateDraw(func() {
				mu.Lock()
				defer mu.Unlock()
				incrementCycle()
				if currentCycle%displayMod == 0 {
					updateDisplay()
				}
				if currentCycle >= DefaultIntervalSize {
					var sidecarSnapshot SidecarInterval
					if generateSidecarJSONL {
						// Sidecar interval event: capture the just-completed interval before
						// push() rolls live counters into history and resets interval state.
						// The existing TUI cycle remains the timing authority for now.
						sidecarSnapshot = PattyGraph.SidecarSnapshotBeforePush()
					}
					push()
					if generateSidecarJSONL {
						if err := PattyGraph.WriteSidecarJSONL(sidecarSnapshot, ""); err != nil {
							log.Printf("PattyLog jsonl write failed: %v", err)
						}
					}
					PattyGraph.writePendingAlertTransitionsJSONL()
					PattyGraph.clearPendingAlertTransitions()
					resetCycle()
				}
			})
		}
	}()
	if err := PattyGraph.app.Run(); err != nil {
		log.Fatalf("Failed to start tview application: %v\n", err)
	}
}

func relayoutUI() {
	// This could try to be smarter but its supposed to be a lightweight call
	newHeight := PattyGraph.sparkPanelHeight()
	PattyGraph.layout.ResizeItem(PattyGraph.sparklineHistoryView, newHeight, 1)
}

func layoutUI() {
	PattyGraph.sparklineHistoryView.SetDynamicColors(true).SetScrollable(true).SetTextAlign(tview.AlignLeft)
	// Main display layout with top view and bottom three views
	PattyGraph.layout = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(PattyGraph.sparklineHistoryView, PattyGraph.sparkPanelHeight(), 1, false). // Main display view at the top
		AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(PattyGraph.botMatchesView, botsDisplayWidth, 1, false). // yes, 1 wider bc
			AddItem(PattyGraph.wordMatchesView, PattyGraph.wordsMatcher.displayWidth, 1, false).
			AddItem(PattyGraph.refsView, PattyGraph.refsMatcher.displayWidth, 1, false).
			AddItem(PattyGraph.ipsView, PattyGraph.ipsMatcher.displayWidth, 1, false),
			0, 1, false) // Bottom row with 4 columns
	PattyGraph.app.SetRoot(PattyGraph.layout, true).EnableMouse(true)
}

func (m *Monitor) sparkPanelHeight() int {
	h := len(m.matchers) - 1 // plain unadjusted height
	if m.selectedMatcher != nil || m.selectedInterestingMatcher != nil {
		h += 2 // adding two lines for linesource at the end
	}
	if m.selectedInterestingMatcher != nil {
		h += 1 // adding a line for selected interesting sparkline detail
	}
	if PattyGraph.showTicker {
		h += 1 // adding a line for the ticker
	}
	return h
}
func (m *Monitor) sparkPanelHeightOld() int {
	selected := -1
	if m.selectedInterestingMatcher != nil {
		matcher := m.selectedInterestingMatcher
		if matcher.selectedKey != "" {
			stats := matcher.wordFrequency[matcher.selectedKey]
			if stats != nil {
				selected = 2
			} else {
				selected = 2
			}
		}
	} else if m.selectedMatcher != nil {
		selected = 1
	}
	if PattyGraph.showTicker {
		selected++
	}
	return len(m.matchers) + selected
}

func (mon *Monitor) statusLine() string {
	status := strings.Builder{}
	lastMonitorMax := lastMonitorMaxBuf.Latest()
	lastLinesMax := lastLinesBuf.Latest()
	lastLastLinesMax := lastLinesBuf.Penultimate()
	lastBytesMax := lastBytesBuf.Latest()
	lastLastBytesMax := lastBytesBuf.Penultimate()

	status.WriteString(fmt.Sprintf("%4s", trimmedCounts(lastMonitorMax)))

	if lastLinesMax > lastLastLinesMax {
		status.WriteString(upArrow)
	} else if lastLinesMax < lastLastLinesMax {
		status.WriteString(downArrow)
	} else {
		status.WriteString(" ")
	}
	status.WriteString(fmt.Sprintf("%4s", trimmedCounts(lastLinesMax)))

	if lastBytesMax > lastLastBytesMax {
		status.WriteString(upArrow)
	} else if lastBytesMax < lastLastBytesMax {
		status.WriteString(downArrow)
	} else {
		status.WriteString(" ")
	}
	status.WriteString(fmt.Sprintf("%4s", strings.TrimSpace(formatBytes(lastBytesMax))))

	status.WriteString(fmt.Sprintf("%2d", fluxDepth))

	status.WriteString(tabGlyph())
	status.WriteString(fmt.Sprintf("{%d:", pattyPushFactor))
	status.WriteString(fmt.Sprintf("%d.%d.%d",
		mon.wordsMatcher.timeToLive,
		mon.refsMatcher.timeToLive,
		mon.ipsMatcher.timeToLive,
	))
	status.WriteString("}")
	status.WriteString(fmt.Sprintf("%2d", pattyGracePeriod))
	status.WriteString(fmt.Sprintf("%0.1f", pattyScaleFactor))
	result := strings.TrimSpace(status.String())
	if len(result) > 37 {
		return result[len(result)-37:]
	}
	return result
}

func colorText(s string, color string) string {
	var b strings.Builder
	for _, r := range s {
		b.WriteString(fmt.Sprintf("[%s]%c", color, r))
	}
	return b.String()
}
func gradientText(s string, colors []string) string {
	var b strings.Builder
	for i, r := range s {
		color := colors[i%len(colors)]
		b.WriteString(fmt.Sprintf("[%s]%c", color, r))
	}
	return b.String()
}

// strip square brackets and resolve to hex for sidecar/display metadata
func tcellColorToHex(raw string) string {
	name := strings.Trim(raw, "[]")
	color := tcell.GetColor(name) // works for both named and #hex values

	r, g, b := color.RGB()
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}
