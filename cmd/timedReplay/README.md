# timedReplay

`timedReplay` is a development and forensic helper for replaying an existing
NGINX access log into a live-looking `access.log`. It is not part of the normal
`pattyGraph` user interface or help output.

The usual workflow is:

1. Use `log_split.sh` to split a captured log into a seed segment and a replay
   segment.
2. Copy the seed segment into place as the current `access.log`.
3. Run `timedReplay` against the replay segment so new lines are appended over
   time.

That lets `pattyGraph` start with a realistic amount of existing log history,
then observe additional traffic arriving at roughly the same per-second cadence
as the original capture.

## Split a log

Brute-force splitting is usually visible in pattyGraph. If the replay segment
starts mid-line or at an unnatural boundary, the split can produce misleading
artifacts that look more important than they are.

From the repository root:

```bash
cmd/timedReplay/log_split.sh 200 access.log.full
```

This creates:

```text
access.log.full.seed
access.log.full.replay
```

The seed size is given in megabytes. The splitter moves forward to the next
newline before cutting so the replay file starts on a complete log line.

## Seed and replay

Use the seed file as the initial access log:

```bash
cp access.log.full.seed access.log
```

Then replay the remaining lines:

```bash
go run ./cmd/timedReplay -file access.log.full.replay >> access.log
```

By default, `timedReplay` writes to stdout. Redirecting stdout is intentionally
the simplest way to append to a watched log. You can also use `-out access.log`
to have the tool open the output file directly in append mode.

## Timing model

`timedReplay` parses NGINX timestamps in the form:

```text
[02/Jan/2006:15:04:05 -0700]
```

It groups all lines from the same log second, writes the whole group, flushes,
then sleeps before writing the next second's group. This preserves bursts within
one second without trying to reconstruct sub-second timing that is not present
in standard NGINX access logs.

Useful flags:

```bash
go run ./cmd/timedReplay -file access.log.full.replay -speed 2.0
go run ./cmd/timedReplay -file access.log.full.replay -out access.log
go run ./cmd/timedReplay -file access.log.full.replay -sync0 >> access.log
```

`-speed` is a multiplier. `2.0` replays two log seconds per real second. This
feature is more of a brute-force feature for development and log investigation.

`-sync0` waits before the first write until the wall-clock second matches the
first valid log line's seconds field. pattyGraph does not rewrite captured log
timestamps as current time, but some live views naturally compare log-time
arrival against wall-clock buckets. During forensic playback, especially because
pattyGraph is good at surfacing unusual timing, starting the replay on an
arbitrary second can create odd-looking split artifacts. Keeping the replay's
log-second cadence aligned with wall-clock seconds makes the playback look more
like live traffic without changing the captured timestamps.

## Caveats

This tool expects input lines to be in timestamp order. Unparsable lines are
dropped with a warning to stderr. It is designed for local development,
demonstration, and investigation against captured logs, not as a general log
shipping or production replay system.
