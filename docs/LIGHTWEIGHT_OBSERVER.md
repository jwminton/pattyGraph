# Lightweight Observer

PattyGraph is meant to be safe to start when things are already going wrong,
and boring enough to leave running when things are not.

During an incident, the observer should not become the next load problem.
PattyGraph is designed to give useful live visibility into an access log without
adding a significant new resource drain to the same machine that is serving
traffic.

That same design also makes PattyGraph suitable as a lightweight server gauge:
something you can leave open in an SSH or tmux session for hours or days while
it quietly tracks traffic shape.

The goal is not "zero cost." Reading, parsing, and rendering live logs always
cost something. The goal is bounded, understandable overhead that is small
enough to justify using PattyGraph both in emergencies and during normal watch
sessions.

## Emergency Startup Goal

The operating assumption is:

```text
If production is already stressed, the observer must stay light.
```

PattyGraph should be practical to start from a shell or tmux session while a
system is under investigation. It tails the existing access log, builds compact
traffic signals, and renders a terminal UI. It does not require adding an
external database, shipping the full log stream elsewhere, or starting a heavy
dashboard stack. 

PattyGraph shows the up-to-the-second view of what is happening in the log files ***now*** and not **five minutes after** a potential bot-army swarm has already started and finished their planned hit-and-run.

This matters because emergency tooling has to be judged by more than what it
can see. It also has to be judged by what it costs to turn on.

## Server Gauge Mode

PattyGraph is also useful when nothing is on fire. Get to know your baseline traffic at a glance.

Because it is local, terminal-based, and built around compact rolling state, it
can sit in a long-running SSH or tmux session as a server gauge. In that mode,
PattyGraph is not an incident response tool you launch once and close. It is a
low-friction operational window you can glance at throughout the day, a server gauge. 

The same traits help both use cases:

- fast startup from recent log context
- bounded memory-oriented state
- compact terminal rendering
- low ceremony to start or stop
- no required external service
- no mandatory log shipping path

Emergency startup and long-running watch mode are two sides of the same design.
PattyGraph tries to stay light enough for both.

## What PattyGraph Avoids

PattyGraph intentionally avoids several common sources of operational weight:

- no required server process
- no required web UI
- no required database
- no required Elasticsearch/OpenSearch indexing path
- no required metrics collector
- no required log shipper
- no required remote AI ingestion

Those tools can still be useful in a broader observability stack. PattyGraph's
role is different: give immediate local visibility from the log file already on
disk.

## Regex Avoidance In The Hot Path

PattyGraph can look like a tool that must be running a lot of regex. It parses
NGINX-style access logs, identifies bot-like User-Agents, extracts request and
referer tokens, tracks IPs, and maintains user-created matchers.

In normal operation, that work is deliberately not regex-driven.

Regex engines are powerful, but they are expensive machinery for a hot path that
runs once per log line. PattyGraph's common path uses predictable string
parsing, token splitting, prefix checks, and predicate matchers instead. Bot
detection, for example, is built around fast User-Agent scanning rather than a
large expression evaluated against every line.

The point is not that regex is bad. The point is that line-rate observation
should not pay regex cost when the log format is predictable and the question is
operational signal extraction.

Optional regex matcher support can still exist for specialized cases. It is not
the design center of PattyGraph's normal access-log pipeline.

## Bounded Startup Context

At startup, PattyGraph can preload a bounded tail of the log with `--read`.

```bash
pattyGraph --read 50 /path/to/access.log
```

That preload is intentionally measured in recent megabytes, not "read the whole
archive." It gives the TUI recent interval history without turning startup into
a full indexing job.

Use smaller values when the host is hot and the live picture matters most:

```bash
pattyGraph --read 10 /path/to/access.log
```

Use larger values when the machine has headroom and recent history matters more.

Of all resources used at startup and while the JSONL content is compact, startup read bounding is probably more of a concern when it comes to JSONL startup output than it is about memory or cpu cycles.

## Compact State Instead Of Raw Retention

PattyGraph keeps compact operational state:

- rolling matcher histories
- top words, refs, and IPs
- IP prefix groups
- byte, line, and error summaries
- selected source examples
- alert state

This is the pattyGraph sliding window of concern. It does not try to keep every raw line in memory. The raw log remains the source of truth on disk. PattyGraph keeps enough state to show traffic shape and a per-retained-token point back to the important raw evidence.

## Allocation Awareness

The hot path is written with allocation pressure in mind. PattyGraph uses
scratch buffers, string interning, compact histories, and reusable series
buffers in places where log volume would otherwise create avoidable garbage and avoidable memory allocations.

That matters because garbage collection is not just a memory concern. During a
live event, avoidable allocation can become CPU noise. During a long-running
watch session, avoidable allocation can become slow drift. PattyGraph tries to
keep the steady-state path simple enough that the runtime is not constantly
cleaning up after the observer.

## Simple Local Concurrency

PattyGraph's internal runtime model is deliberately simple:

- one path tails the access log
- one optional path tails the control file
- one path updates the TUI
- shared state is coordinated inside the process

That is not distributed or parallel processing. It is a local terminal monitor. The simpler
shape is part of the resource story: fewer moving pieces, fewer processes, and
less operational surface area while debugging or watching a live system.

## Built-In Resource Visibility

PattyGraph also exposes its own runtime behavior through factoids. Memory and GC
factoids can show values such as heap usage, allocation counts, GC cadence, and
reuse rates.

That means the observer can inspect itself while it is running. If PattyGraph is
being used during a high-pressure session, its own resource behavior is visible
inside the same terminal workflow.

Examples of resource-oriented fact categories include:

```text
mem.heap
mem.allocs
mem.wsReuse
metrics.pool
```

## What To Watch In Practice

When starting PattyGraph on a stressed host and absolute minimalism is required, prefer conservative settings:

```bash
pattyGraph --read 10 /path/to/access.log
```

Add `--json` only when the structured sidecar is useful:

```bash
pattyGraph --json --read 10 /path/to/access.log
```

Use larger `--read` windows when the host has enough headroom or when recent
history is more important than the smallest possible startup footprint.

The main practical checks are:

- Does startup complete quickly?
- Does the TUI remain responsive?
- Is the host already CPU or I/O saturated?
- Is `--json` output needed for this session?
- Is the selected `--read` window larger than the situation requires?
- Is this a short emergency view or a long-running watch session?

## Not A Replacement For Capacity Planning

PattyGraph being light does not mean it is magic. If a machine is completely
I/O-bound, reading more log data still costs I/O. If a log line format is
pathological, parsing still costs CPU. If `--read` is set too high, startup
will do more work.

The design intent is that PattyGraph gives operators a low-friction first look
without requiring a new heavy subsystem at the moment of need, and without
becoming noisy when left open as a long-running terminal gauge.

## Short Version

PattyGraph is built to be an emergency-friendly observer and a lightweight
server gauge: local, terminal-based, bounded by default, careful about hot-path
costs, and focused on compact signals instead of full raw-log retention. It
should be reasonable to start during an incident or leave running in an SSH
session without turning observation into a significant new resource drain.
