# Changelog

## 0.1.6

Improves PattyGraph's live usability and operational feedback after `v0.1.5`.

- Added an in-TUI quick help panel for terse key and mouse reminders.
- Added cleaner JSONL and control-file configuration.
- Added safer handling for repeated output failures.
- Added more named factoids and operator-visible output feedback.
- Improved Bots promotion behavior and matcher interaction help.
- Expanded internal documentation around the configuration, inline command,
  factoid, and matcher systems.

## 0.1.5

Focused on cleanup, internal clarity, and operator ergonomics after the alerting
release.

- Added the `Ctrl-H` quick help panel for terse in-TUI keyboard and mouse hints.
- Added `--json-file` for stable PattyLog JSONL filenames under `<save-dir>`.
- Removed stale experimental sparkgraph JSON output behavior in favor of
  PattyLog JSONL.
- Improved matcher, time-pressure, WordStats, inline-command, and factoid source
  comments so the settled upon runtime behavior is easier to review.
- Reorganized selected source files to make core monitor and matcher concepts
  easier to find.
- Fixed Bots startup/replay promotion behavior and added regression coverage.
- Added or refined documentation for signal-first operation, lightweight
  observation, click zones, bot-army detection, time pressure, and traffic speed.

## 0.1.4

Alerting became part of the normal operational loop.

- Added matcher alerts that can trigger when counts go above or below a
  configured threshold.
- Added inline alert commands for configuring, inspecting, listing, and clearing
  alert state from the live command surface.
- Persisted alert configuration through config output so tuned thresholds can be
  replayed or restored.
- Added alert transition records to the PattyLog JSONL sidecar stream, including
  triggered and recovered states.
- Bumped the PattyLog JSONL sidecar schema version to `4` for the `v0.1.4`
  release.
- Added tests for alert commands, alert persistence, quoted matcher names, and
  sidecar alert event shape.
- Promoted `timedReplay` as a small companion command under `cmd/timedReplay`
  for development and forensic replay of captured NGINX access logs.
- Added `cmd/timedReplay/log_split.sh` and scoped local documentation for the
  seed-and-replay workflow used during log investigation.

## 0.1.3

Closed the control loop for assisted operation.

- Expanded embedded help into topic-specific operational guidance, including
  inline commands, JSONL sidecar behavior, layout, colors, words, facts, and AI
  workflow notes.
- Documented an AI-assisted workflow using PattyLog JSONL for observation and
  `pattyControl.log` for command input.
- Added sidecar control-command records so command attempts and results are
  visible in the same stream as interval state.
- Added startup metadata and control-file markers that make it easier for a
  human or AI operator to discover the active session, watched log path, control
  file path, and sidecar output path.
- Improved command/help text so PattyGraph can explain its live control surfaces
  without relying on external notes.

## 0.1.2

Introduced the sidecar observation stream.

- Added PattyLog JSONL sidecar output for interval summaries, matcher counts,
  interesting keys, grouped IP observations, selected context, factoids, and
  startup metadata.
- Added schema-versioned sidecar records for machine-readable inspection outside
  the terminal UI.
- Added options and help for controlling sidecar output from the CLI.
- Improved sidecar coverage with tests for schema versioning, startup records,
  matcher summaries, interesting value tracking, selected context, and write
  behavior.
- Kept the sidecar focused on shared situational awareness instead of replacing
  the live terminal display.

## 0.1.1

Solidified the control surfaces and put real tests around the core behavior.

- Added a focused Go test suite covering parser behavior, buffers,
  tokenization, matcher behavior, monitor pipeline flow, inline commands,
  config/output handling, and control-file gating.
- Added optional `pattyControl.log` command input support, with CLI and inline
  controls for enabling and disabling processing from the control file.
- Made inline command behavior testable enough to support live tuning through
  commands and config replay.
- Allowed NGINX access log lines with a trailing `"-"` field after the
  User-Agent while preserving the standard User-Agent token space.
- Hardened User-Agent parsing so trailing whitespace and the supported trailing
  field do not pollute User-Agent tokenization.
