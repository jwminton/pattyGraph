# PattyGraph
Stop dumping raw server logs into expensive LLMs. Let an AI agent read a compressed JSONL sidecar while you watch the live TUI dashboard
![PattyGraph terminal UI](docs/images/pattyGraph-startup.gif)

## Startup Speed (aka: 'Instant On')
PattyGraph is designed to become useful almost immediately.

On a typical development machine, PattyGraph can read and summarize about 80 MB of existing NGINX access log in roughly a second which is typically around 200K log lines. This is pattyGraph consuming that data as though it was live at its fastest full processing speed minus the TUI updates. This isn't a crippled startup mode. Once started, the TUI and PattyLog have the recent context to show traffic shape before live tailing takes over, as though pattytGraph had been running the whole time. A fully operational view starting from zero to ready in one second.

## New in PattyGraph

pattyGraph has expanded to express a more complete operational loop: observe, control, record, and alert.

- `--help ai` documents the intended AI-assisted operation workflow for agents,
  automation, and human operators.
- Recommended assisted sessions run PattyGraph under tmux with PattyLog JSONL
  enabled through `--json`.
- `--control` and `pattyControl.log` provide a live command path for inline
  commands.
- Matcher alerts watch for counts above or below configured thresholds.
- Alert configuration is persisted through config output so tuned thresholds can
  be replayed or restored.
- Individual Alert comments are preserved with the alert creation: <br>`!!! alert health below 20   # Missing health checks?`


## Tell me more...

PattyGraph is a real-time terminal access-log analyzer for live ops, bot discovery, and traffic forensics. Pattygraph incluides sidecar JSONL output, so the same run that drives the interactive TUI can also write structured interval records for scripts, replay workflows, and AI-assisted triage. Use the terminal view to see traffic shape as it happens, then use the sidecar stream to decide where to aim `rg`, `grep`, `awk`, or deeper raw-log inspection.


PattyGraph is a terminal-based, real-time access log analyzer for nginx-style logs. It highlights unusual or significant traffic patterns using sparklines, matchers, and ranked token/referrer/IP tables.

It’s designed for live ops use (tmux/screen) and forensics (replaying historical log windows), with a dense interactive display that helps you see traffic **shape** and how it changes over time.


## Download

Prebuilt Linux binaries are available from the PattyGraph 0.1.4 release page:

- [pattyGraph v0.1.4 release](https://github.com/jwminton/pattyGraph/releases/tag/v0.1.4)


## Features

- **Live traffic dashboard**: sparklines + interval-based stats over a rolling window
- **Matchers**: track known patterns and promoted sources (bots/scrapers/etc.)
- **Token tracking**: interesting URI/User-Agent tokens, referrers, and IPs
- **User-Agent analysis**:
  - residue buckets (post-cleanup token-count signature)
  - per-IP User-Agent drift (token-based distance)
- **Interactive UI**: clickable sparklines, selectable matchers, cross-highlighting, per-entry history sparklines
- **Inline commands** (`!!!`): runtime control and config injection through the log stream
- **Timed replay support**: `cmd/timedReplay` replays logs with original timing shape for demos/testing/forensics

## Live Terminal Triage, Now with JSONL Sidecar Output

Let pattyGraph tell your AI tools where to start with your next NGINX log emergency!

This release turns PattyGraph into something more than a live terminal viewer for NGINX-style access logs. It is now designed to be invoked by AI tools, scripts, and automation workflows as a first-pass log investigation layer.

PattyGraph still runs as an interactive TUI for humans watching traffic in real time, but the new `-j` / `--json` mode writes a sidecar JSONL stream alongside it.

That sidecar gives another AI or automation process structured interval records: active matchers, top IPs, interesting URI and user-agent tokens, refs, bot activity, error bursts, IP groups, traffic totals, and generated factoids.

The practical goal is simple:

An AI should not have to ingest an entire access log just to figure out what is happening.

PattyGraph can give it the shape of the traffic first. Then the AI can decide what raw-log searches to run next, which IPs or paths deserve attention, whether bot activity is normal or suspicious, and where deeper investigation should begin.

## Quick Start

Build all targets (writes into `dist/`):

```bash
./compile.sh
````

Run (defaults to `./access.log` if no file is given):

```bash
./dist/linux-amd64/pattyGraph
# or
./dist/linux-amd64/pattyGraph /var/log/nginx/access.log
```

Helpful sub-help:

```bash
./dist/linux-amd64/pattyGraph --help
./dist/linux-amd64/pattyGraph --help layout
./dist/linux-amd64/pattyGraph --help inline
./dist/linux-amd64/pattyGraph --help colors
```

## Visual Diagnosis

![PattyGraph composite examples](docs/images/pattyGraph-states-2x2.png)

At a glance, the scope and urgency of failures can be categorized. Looking left to right, top row first: 
- **Normal Startup**: Maybe some errors but nothing persistent. No real pattern to the red error highlights
- **Potentially bad clients**: More errors and there are some persistent IP's or IP ranges that are the source.
- **Potentially bad deployment**: Errors are more related to the content being hit than the clients doing the requesting and the error spread may be wider, but upon investigation, common deployment characteristics can be seen
- **Systemic error**: Service itself might be down or there is some fatal root error causing a system-wide issue


## timedReplay

`timedReplay` is a small companion utility that replays captured NGINX access
logs with controlled timing, letting PattyGraph watch historical traffic as if
it were arriving live.

A typical replay session uses two shells. Start PattyGraph on an output log:

```bash
touch replayed_access.log
./pattyGraph replayed_access.log
```

Then append replayed traffic from another shell:

```bash
go run ./cmd/timedReplay -file ./access.1.log >> ./replayed_access.log
```

`timedReplay` groups lines by their NGINX timestamp second, preserving bursts
from the original capture.

For larger investigations, `cmd/timedReplay/log_split.sh` can split a capture
into a seed file and replay file. See `cmd/timedReplay/README.md` for the full
workflow.


## Inline Commands

Lines beginning with `!!!` are interpreted as commands rather than log lines. This is used for runtime control and for configuration files (a config file is just a sequence of inline commands). 

Example:

Adds a new Matcher looking for the simple text "Applebot" in any part of the log line.

```bash
echo '!!! add Applebot' >> access.log
```

See:

```bash
./dist/linux-amd64/pattyGraph --help inline
```

## Documentation
(Planned and existing)

* [Startup Speed aka:"Instant On"](./docs/STARTUP_SPEED.md)
* [The Lightweight Observer](./docs/LIGHTWEIGHT_OBSERVER.md)
* [PattyLog JSONL: Live Shape](./docs/JSONL_DIAGRAM.md)
* [TUI Tab view cycle](./docs/TAB_VIEW_CYCLES.md)
* [Tokenized User-Agent Levenschtein Distance](./docs/LEVENSHTEIN_DISTANCE.md)
* [TUI Mouse Interaction and Click Zones](./docs/CLICK_ZONES.md)
* [Selection Deep Dive](./docs/SELECTION_DEEP_DIVE.md)
* Traffic texture model: `docs/traffic-texture.md`
* User-Agent residue buckets: `docs/user-agent-residue-profiling.md`
* Architecture notes: `docs/architecture.md`

## Sample Data Policy

This repository does **not** distribute real access log data. Use logs you own/administer/are authorized to inspect. Screenshots may be taken from authorized or public datasets, but raw logs are not hosted in the repo.

## License

Apache-2.0. See [LICENSE](LICENSE).

```
