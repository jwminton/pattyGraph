// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/pflag"
)

// flags.go owns PattyGraph's command-line surface: flag metadata, generated
// usage/help output, argument parsing, and MonitorConfig construction. Runtime
// flag effects are also reused by config replay and inline commands, so this
// file is intentionally metadata-driven instead of scattering pflag calls.

// flagInfo describes one CLI flag and the type-specific map used to retrieve it
// after pflag parses command-line input.
type flagInfo struct {
	Name         string
	Short        string
	DefaultValue interface{}
	Description  string
	FlagType     string // "int", "string", "float64", "bool", "stringArray"
}

// flags is the canonical command-line metadata table. These names are reused by
// usage category rendering, config replay, inline command handling, and final
// MonitorConfig creation.
var flags = []flagInfo{
	{"title", "t", "", "Set title for display labeling (defaults to machine name)", "string"},
	{"save-dir", "d", "", "Directory to save configs & splats", "string"},
	{"config", "c", "", "Config file", "string"},
	{"scale", "s", pattyScaleFactor, "Interest Scale factor to be more 'interesting' and trackable. (0.1-4.0)", "float64"},
	{"push", "p", pattyPushFactor, "Push factor to purge items of interest from 'interesting' columns (0-11)", "int"},
	{"grace", "g", pattyGracePeriod, "Grace period (in intervals) before entries can qualify for permanent interest.", "int"},
	{"flux", "f", DefaultFluxDepth, "Flux is the history depth used for interesting word ranking.", "int"},
	{"read", "r", DefaultMBToRead, "Number of MB back of logfile to read upon startup", "int"},
	{"color-index", "l", 0, "Advance the color assignment index for a different look", "int"},
	{"json", "j", false, "Write PattyLog JSONL interval data to <save-dir>/pattyLog_<date>_<time>_<pid>.jsonl", "bool"},
	{"json-file", "", "", "Write PattyLog JSONL to a specific file under <save-dir>; implies --json", "string"},
	{"control", "C", false, "Read inline commands from pattyControl.log in save-dir/current dir", "bool"},
	{"control-file", "", "", "Read inline commands from a specific file under <save-dir>; implies --control", "string"},
	{"zero", "0", false, "Force cycle count = 0 at start", "bool"},
	//{"expert", "x", false, "Start with expert display on", "bool"},
	{"version", "v", false, "Prints current version and exits", "bool"},
	{"help", "h", false, "Print this message or '-h <ai|layout|jsonl|inline|colors|words|facts>' for detailed help", "bool"},
}

type MonitorConfig struct {
	filePath            string
	excludeRequest      bool
	saveDir             string // expanded save directory, e.g. /home/user/patty
	saveDirOriginal     string // user-provided save directory, e.g. ~/patty
	jsonFile            string // resolved PattyLog JSONL output path
	jsonFileOriginal    string // user-provided PattyLog JSONL output path
	controlFile         string // resolved control file path
	controlFileOriginal string // user-provided control file path
	builtinConfFile     string
	mbToRead            int
}

func (c *MonitorConfig) setSaveDir(value string) {
	c.saveDirOriginal = value
	c.saveDir = expandUser(value)
	c.refreshJSONFilePath()
	c.refreshControlFilePath()
}

func (c *MonitorConfig) setJSONFile(value string) {
	c.jsonFileOriginal = value
	c.refreshJSONFilePath()
}

func (c *MonitorConfig) setControlFile(value string) {
	c.controlFileOriginal = value
	c.refreshControlFilePath()
}

func (c *MonitorConfig) refreshJSONFilePath() {
	if c.jsonFileOriginal == "" {
		c.jsonFile = ""
		return
	}
	expanded := expandUser(c.jsonFileOriginal)
	if filepath.IsAbs(expanded) || c.saveDir == "" {
		c.jsonFile = expanded
		return
	}
	c.jsonFile = filepath.Join(c.saveDir, expanded)
}

func (c *MonitorConfig) refreshControlFilePath() {
	if c.controlFileOriginal == "" {
		c.controlFile = ""
		return
	}
	expanded := expandUser(c.controlFileOriginal)
	if filepath.IsAbs(expanded) || c.saveDir == "" {
		c.controlFile = expanded
		return
	}
	c.controlFile = filepath.Join(c.saveDir, expanded)
}

func printVersion() string {
	return fmt.Sprintf("  %s %s by %s\n%s\n", PattyGraphName, PattyGraphVersion, PattyGraphAuthor, PattyGraphGithubUrl)
}

func parseArgs() *MonitorConfig {
	mConf := &MonitorConfig{
		//intervalLength: DefaultIntervalSize,
	}

	intMap := make(map[string]*int, 10)
	boolMap := make(map[string]*bool, 10)
	stringMap := make(map[string]*string, 10)
	floatMap := make(map[string]*float64, 10)
	saMap := make(map[string]*[]string, 10)
	// Define pflags using the metadata
	for _, f := range flags {
		switch f.FlagType {
		case "int":
			intMap[f.Name] = pflag.IntP(f.Name, f.Short, f.DefaultValue.(int), f.Description)
		case "string":
			stringMap[f.Name] = pflag.StringP(f.Name, f.Short, f.DefaultValue.(string), f.Description)
		case "float64":
			floatMap[f.Name] = pflag.Float64P(f.Name, f.Short, f.DefaultValue.(float64), f.Description)
		case "bool":
			boolMap[f.Name] = pflag.BoolP(f.Name, f.Short, f.DefaultValue.(bool), f.Description)
		case "stringArray":
			saMap[f.Name] = pflag.StringArrayP(f.Name, f.Short, f.DefaultValue.([]string), f.Description)
		default:
			panic(fmt.Sprintf("Unsupported flag type: %s", f.FlagType))
		}
	}

	pflag.Usage = func() {
		fmt.Print(printVersion())

		args := pflag.Args()
		if len(args) == 1 && args[0] == "colors" {
			fmt.Println(colorHelpText())
			dumpColors() // your custom function
			return
		}
		if len(args) == 1 && args[0] == "facts" {
			fmt.Println(factHelpText())
			dumpFacts() // your custom function
			return
		}

		if len(args) == 1 && args[0] == "inline" {
			fmt.Println(inlineCommandHelp())
			return
		}

		if len(args) == 1 && args[0] == "layout" {
			fmt.Println(terminalScreenHelp())
			return
		}

		if len(args) == 1 && args[0] == "words" {
			fmt.Println(printBuiltinWordLists())
			return
		}

		if len(args) == 1 && (args[0] == "jsonl" || args[0] == "json") {
			fmt.Println(sidecarHelpText())
			return
		}

		if len(args) == 1 && args[0] == "ai" {
			fmt.Println(aiHelpText())
			return
		}

		fmt.Println(usageText())

		categories := map[string][]string{
			"General Settings": {"push", "scale", "grace", "flux"},
			//"Customization":    {"title", "color-index", "read", "expert", "zero"},
			"Configuration": {"config", "save-dir", "json", "json-file", "control", "control-file"},
			"Customization": {"title", "color-index", "read", "zero"},
			"Help":          {"help"},
		}
		categoryNames := []string{"General Settings", "Configuration", "Customization", "Help"}

		for _, category := range categoryNames {
			flagNames := categories[category]
			fmt.Printf("%s:\n", category)
			for _, flagName := range flagNames {
				for _, f := range flags {
					if f.Name == flagName {
						defaultText := ""
						typeDescription := f.FlagType
						if f.DefaultValue != nil {
							// smooth out the autotext here
							if f.FlagType == "stringArray" {
								typeDescription = "string"
							} else if f.FlagType == "bool" {
								typeDescription = ""
							} else if f.FlagType == "string" && f.DefaultValue == "" {
								// Skip showing default for empty strings
							} else {
								defaultText = fmt.Sprintf(" (default %v)", f.DefaultValue)
							}
						}
						if typeDescription != "" {
							typeDescription = fmt.Sprintf("<%s>", typeDescription)
						}
						flagPrefix := fmt.Sprintf("  -%s, --%s", f.Short, f.Name)
						if f.Short == "" {
							flagPrefix = fmt.Sprintf("      --%s", f.Name)
						}
						fmt.Printf("%s %s%s\n", flagPrefix, typeDescription, defaultText)
						fmt.Printf("        %s\n", f.Description)
					}
				}
			}
		}
	}

	pflag.Parse()

	doHelp := boolMap["help"]
	if *doHelp {
		pflag.Usage()
		os.Exit(0) // You decide the exit behavior
	}
	doVersion := boolMap["version"]
	if *doVersion {
		fmt.Println(printVersion())
		os.Exit(0) // You decide the exit behavior
	}

	// Preserve the configured path so startup can replay the same config source
	// after flag parsing finishes.
	mConf.builtinConfFile = *stringMap["config"]

	// Each type was added to the corresponding map for retrieval here
	mConf.setSaveDir(*stringMap["save-dir"])
	mConf.setJSONFile(*stringMap["json-file"])
	mConf.setControlFile(*stringMap["control-file"])
	mConf.mbToRead = *intMap["read"]

	// legacy: globals that were getting set at parse time
	pattyGracePeriod = *intMap["grace"]
	fluxDepth = *intMap["flux"]
	pattyPushFactor = *intMap["push"]
	pattyScaleFactor = *floatMap["scale"]
	forceZeroStart = *boolMap["zero"]
	//expertMode = *boolMap["expert"]
	generateSidecarJSONL = *boolMap["json"] || mConf.jsonFile != ""
	enableControlFile = *boolMap["control"] || mConf.controlFile != ""
	colorIndex = *intMap["color-index"] // from config
	machineDisplayName = *stringMap["title"]
	machineDisplayName = *stringMap["title"]

	// Flag Validation
	// Validate push factor
	if pattyPushFactor < 1 || pattyPushFactor > 11 {
		panic(fmt.Sprintf("Invalid push factor: %d. Valid values are 1-11.", pattyPushFactor))
	}
	// Validate scale factor
	if pattyScaleFactor < 0.1 || pattyScaleFactor > 4.0 {
		panic(fmt.Sprintf("Invalid scale factor: %.2f. Valid values are 0.1-4.0.", pattyScaleFactor))
	}
	// look to the OS now if no title given
	if machineDisplayName == "" {
		tmp, err := os.Hostname()
		if err != nil {
			tmpError := "--error--"
			machineDisplayName = tmpError // Assign pointer to the error string
		} else {
			machineDisplayName = tmp // Assign pointer to the hostname string
		}
	}

	args := pflag.Args()
	if len(args) > 0 {
		mConf.filePath = args[0] // Use the first non-flag argument as the file
	} else {
		// Default to local access log file if none provided
		mConf.filePath = defaultLogFilename
	}
	return mConf
}

func usageText() string {
	return `
Usage: ./pattyGraph [OPTIONS] [file_location]

  pattyGraph is a real-time terminal-based log analysis tool that highlights unusual 
  or significant patterns in standard-format access logs. It parses each line for key 
  fields like user agents, referrers, IP addresses, and request URIs, and 
  visualizes activity using sparklines, dynamic token tables, and interval-based 
  statistics. The interface is designed for live terminal interaction (typically run 
  in tmux or screen), giving you a structured view into what’s happening across your 
  system's traffic in real time.

  If [file_location] is not specified, ./access.log is assumed. Standard web logging 
  formatting (e.g. nginx standard format) is assumed.

  Log data is tracked across a rolling 80-interval window (60 seconds per interval), 
  allowing trends to emerge over time. Frequent or high-volume entries are 
  automatically promoted to "Peak" status, visually marked in orange and pinned at 
  the top of their respective columns. Peak entries do not expire naturally and must 
  be explicitly purged (e.g. via ctrl-p or the !!! purge inline command).

  The three core tuning controls — push, scale, and grace — shape what qualifies as 
  "Interesting". push sets the time pressure on entries, accelerating how quickly 
  idle data is aged out. scale amplifies or dampens the perceived importance of 
  tokens based on observed frequency. grace defines how long an entry must survive 
  before it becomes eligible for Peak status.

  PattyGraph uses the notion of 'primeFlux' — the sum of the current interval
  and the most recent N historical values (nFlux). InterestingWords are ranked and
  sorted based on a scaled score derived from primeFlux. The default fluxDepth is 3.
  Lower fluxDepth values make the list more reactive to recent spikes. Higher values
  introduce more historical inertia, resulting in steadier sort order and entry retention.

  Words (URI & user-agent tokens) and Refs are evaluated by relative log line 
  frequency, while ips are also measured for bursty behavior across intervals. 
  The goal is to surface noisy clients, aggressive scrapers, or traffic anomalies 
  with only traffic inspection.

  Terminal interface is clickable: matcher entries along the left are selectable as 
  are the interesting column entries, sparkline graphs can be clicked for value 
  inspection. Keyboard-driven actions can also be triggered via inline '!!!'
  commands (see '--help inline'). Configuration files are a sequence of inline
  commands read in prior to log data ingestion.
  
   Ctrl-Select - (Matcher on the left side) Toggles whether to print matched entries
                 with 0 counts that interval. (Useful when Bots has a lot of stale
                 entries with zero count)

Keyboard Shortcuts:
   </> (grace adjustment)
   {/} (flux depth adjustment)
   ctrl-left/ctrl-right (push adjustment) 
   ctrl-down/ctrl-up (scale adjustment)
   [/] (Mini Sparkline sliding window adjustment)
       Decrease/increase secondary info display of interesting entries by 5
   ctrl-m     Add selected entry as matcher (top of list), or pop Bots top match
   ctrl-n     Add selected entry as non-competing matcher (under Bots)
   ctrl-b     Add selected entry as matcher (above Bots)
   ctrl-s     Save screen to <save-dir>/pattySplat_<date>_<time>_<pid>.txt
   ctrl-g     Save config to <save-dir>/pattyGraph_<date>_<time>_<pid>.conf
   ctrl-h     Toggle quick help panel
   ctrl-p     Clear matcher details if selected; otherwise purge Peak entries
   ctrl-d     Delete selected matcher (or disable Bots bot spawning)
   ctrl-f     Toggle random fact stream source
   U/D        Move selected matcher Up/Down 
   tab        Increment secondary information display
   esc        Deselect active selection (one at a time)
   f/F        Freeze "Interesting" display for 5/10 cycles to make selection easier
   x          Toggle expert mode overlay
   X          Toggle Ticker display
   q          Quit
`
}
func factHelpText() string {
	return `
Factoids are short runtime summaries generated to reflect current state,
memory usage, matcher activity, and more.

Control fact display and the random stream with the following inline commands:
  !!! ticker         # Toggle ticker stream display
  !!! facts.rnd      # Toggle random fact stream

Or via keyboard commands:
  'X'       - Toggle Factoid stream display
  Ctrl-F    - Toggle Random Fact stream

Facts may be requested directly by name or allowed to appear randomly during
normal display when the ticker pane is open and the random stream is enabled.

To schedule a factoid to appear immediately in the ticker:
  echo '!!! fact mem.wsreuse'      >> access.log
  echo '!!! fact most.active'      >> access.log
  echo '!!! fact ips.prefixes'     >> access.log

Factoid names are case-insensitive.

`
}

func colorHelpText() string {
	return `
Matcher colors can be specified using one of the following:
  • A named color (from the X11 color naming standard)
  • A hex color in #RRGGBB format
  • A numeric index into the color table shown below
Examples:
  echo '!!! color Googlebot red'       >> access.log
  echo '!!! color Googlebot #FF0000'   >> access.log
  echo '!!! color Googlebot 27'        >> access.log

A color index is maintained for automatic matcher color assignment.
This index can be set via command-line option or changed at runtime:

  echo '!!! color-index 27' >> access.log
`
}
func inlineCommandHelp() string {
	return `
Inline command reference for config injection and runtime interaction. Any
log line beginning with '!!!' is interpreted as an inline command instead
of normal log content. Generated configuration files are a sequence of
inline commands read in before data ingestion.

Inject lines to manage pattyGraph operation:
  echo '!!! purge' >> access.log

NOTE: Single quotes avoid shell command expansion

Matcher Commands:
  !!! add <flag*> <name> <pattern>...
        valid flags: --refs, --words, or --ips
      Add a matcher with one or more text patterns. Quoted patterns allowed.
      If a flag is given that 'interesting' scope is used. If no flag is given
      the entire log line is searched.
  !!! del <name> 
      Delete a matcher (text after <name> is ignored).
  !!! color <name> <color>
      Assign a named matcher a specific display color. See '--help colors'.
  !!! mode <name> <0|1|2>
      Set named matcher match information expansion from 0-minimal to 2-all
  !!! select
      Clear the current interesting-item selection.
  !!! select <flag> <key>
        valid flags: --refs, --words, or --ips.
      Select the first matching interesting item in the given scope. Exact match.
      A flag must be used to indicate scope
  !!! alert <name> above <count>
      Trigger when matcher count is count or higher for flux-depth consecutive
      intervals. Recovers after flux-depth consecutive intervals below count.
  !!! alert <name> below <count>
      Trigger when matcher count is below count for flux-depth consecutive
      intervals. Recovers after flux-depth consecutive intervals at or above count.
      below 1 is the way to alert on zero hits.
  !!! alert <name>
      Show current alert settings and state for a matcher.
  !!! alert <name> clear [above|below]
      Clear both alert bounds or only the requested bound.
  !!! alerts
      Report currently triggered alert bounds across all matchers.

Alert Notes:
  Alerts attach simple bounds to existing matchers. They evaluate once per
  interval when matcher counts are pushed.

  above N means N or more hits in an interval.
  below N means fewer than N hits in an interval.

  A bound must be true for flux-depth consecutive intervals before it triggers.
  A triggered bound must be false for flux-depth consecutive intervals before it
  recovers. Manual clear resets alert state and is recorded as a control command;
  it does not emit a recovered alert event.

  Each matcher can have one above bound and one below bound. If both are set,
  the below threshold must be less than or equal to the above threshold.

  Alert transitions are written to PattyLog JSONL as event_type "alert" records
  with status "triggered" or "recovered".

Examples:
  !!! alert errs above 50
      Alert when errs reaches 50 or more for flux-depth consecutive intervals.
  !!! alert Googlebot below 1
      Alert when Googlebot has zero hits for flux-depth consecutive intervals.
  !!! alert Googlebot above 500
      Also alert if Googlebot reaches 500 or more hits.
  !!! alert Googlebot
      Show Googlebot alert configuration and runtime state.
  !!! alert Googlebot clear above
      Remove only the above bound.


Matcher names can be prefixed with a modifier to control placement:
  +name → Add matcher at the top of the list.
  -name → Add matcher just above the built-in "Bots" matcher. (default)
  *name → Add as a non-competing matcher after "Bots".
Modifiers apply to 'add', and are accepted but ignored for 'color' and 'del'.

Configuration Settings:
  !!! push <int>
  !!! grace <int>
  !!! scale <float>
  !!! title <name>
  !!! save-dir <path>
      Set the directory for config and splat output (e.g. ~/splats).
  !!! json-file <path>
      Write PattyLog JSONL to a specific file relative to <save-dir>. Implies json on.
  !!! control <on|off>
      Enable or disable pattyControl.log command input. Use -C/--control to start with it on.
  !!! control-file <path>
      Save a control-file path for generated config. Active control tailer changes require restart.
  !!! color-index <int>
      Index into the color table for Matcher color assignment
Misc Commands:
  !!! popBots
      Forces Bots to fork its top Bot match as new matcher
  !!! purge
      Clears all peak words
  !!! demo
      Advances tab view once every 10 seconds. Tab stops progression.
  !!! ticker
      Toggle Ticker operation (same as keyboard 'X') 
  !!! history
      Toggle Factoid History display (same as selecting Ticker) 
  !!! fact <fact.name>
      Injects named factoid as the next factoid. See '--help inline'
Persistence & Export:
  !!! pattySplat
      Save the current screen state to a timestamped file.
  !!! dumpConfig
      Save the current matcher definitions to a new timestamped config file.
  !!! json <on|off>
      Turn PattyLog JSONL output on or off.

General:
  - Lines must begin with '!!! '.
      (When executing from a shell, single quotes avoid shell command expansion) 
  - Quoted arguments are supported within a command:
       e.g. '!!! add FTwitterbot "Facebot Twitterbot"'
  - Lines starting with '#' and lines that don't match '!!!' are ignored.
`
}

func printBuiltinWordLists() string {
	wordlist := strings.Builder{}
	wordlist.WriteString(fmt.Sprintf("\nBuiltin words:\n"))
	wordlist.WriteString(fmt.Sprintf("\n  Common words filtered from words & refs:\n     "))

	const wordsPerLine = 9
	for i, word := range commonWordList {
		wordlist.WriteString(word)
		if (i+1)%wordsPerLine == 0 {
			wordlist.WriteString("\n     ")
		} else {
			wordlist.WriteString(" ")
		}
	}

	// Make sure it ends cleanly with a newline
	if len(commonWordList)%wordsPerLine != 0 {
		wordlist.WriteString("\n")
	}
	wordlist.WriteString("\n  Builtin 'Bots' search patterns:\n     ")
	for _, term := range BotsSearchTerms {
		wordlist.WriteString(fmt.Sprintf("*%s ", term))
	}
	wordlist.WriteString("\n\n  Builtin 'Platform' detection regex pattern:\n     ")
	wordlist.WriteString(platformRegexString)
	wordlist.WriteString("\n\n  Builtin 'Browser' detection regex pattern:\n     ")

	maxWidth := 70
	currentLineLen := 0
	tokens := strings.Split(browsPattern, "|")

	for i, token := range tokens {
		// Keep delimiters in place
		separator := "|"
		if i == 0 {
			separator = ""
		}
		addition := separator + token

		if currentLineLen+len(addition) > maxWidth {
			wordlist.WriteString("\n     ")
			currentLineLen = 0
		}

		wordlist.WriteString(addition)
		currentLineLen += len(addition)
	}

	wordlist.WriteString("\n")

	return wordlist.String()
}

func terminalScreenHelp() string {
	return `
The screen layout includes sparklines, token tracking (words, 
refs, IPs), and active log line matchers on the left.

Layout:
+-----------------------[A] SparkPanel----------------------+
|<file watched>      <title><stats><clicked value><log time>|
| <interval counter><time scale w/highlight><expert overlay>|
| Matcher1 current: first|▂▃▅▆sparkline▆▅▃▂                 |
| Matcher2 current: first|▂▃▅▆sparkline▆▅▃▂                 |
+----------------+---------------+---------------+----------+
| [B] Matchers   | [C] Words     | [D] Refs      | [E] IPs  |
+----------------+---------------+---------------+----------+

Example Expert Overlay (optional top right text overlay):
  20 3000 80M/{4:90.120.180}301.5
Value Decomposition:
  20 - Max value seen in competing matchers (i.e. matchers above & including Bots)
  3000 - Max lines seen per interval
  80M - Max bytes served per interval
  / - Secondary information indicator
      '-' - PattyFactor
      '/' - Current + Flux Depth of History
      '|' - History Depth
      '\' - Average User-Agent edit difference per IP
      '=' - Mini sparkline sliding window view
   (Note: For the sliding window, the time scale above the spark panel will 
          highlight the corresponding tick for the sliding window's relative
          position)
   {4:90.120.180}
      Push factor and resulting purge windows, in seconds, for Words, Refs,
      and IPs respectively. If an entry is not seen again within its window,
      it is purged.
   30 - Grace factor
   1.5 - Scale for Peak Word determination
`
}
