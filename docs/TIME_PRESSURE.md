# Time Pressure

A match only starts the test.

A word, referer, or IP becomes interesting because of its context, repetition,
and survival under time pressure. PattyGraph is constantly asking whether a key
is still active enough, repeated enough, or durable enough to deserve screen
space.

The `words`, `refs`, and `ips` columns are under constant time pressure. A key
can appear, become visible, build history, disappear, or become Peak depending
on whether traffic keeps hitting it often enough. That pressure is what keeps
the interesting columns from turning into a permanent list of everything the log
has ever seen.

The direction of the main control is intentionally simple: a higher `push` value
means stronger pressure to remove individual interesting entries from the live
working set.

This is the operating model underneath the interesting columns, selected items,
secondary views, and Peak entries. PattyGraph reduces raw access-log lines into
live signal by letting repeated context survive and letting stale noise fall
away.

The screen is asking:

```text
what is still active enough to deserve attention?
```

not:

```text
what has ever appeared in the access log?
```

## Controls At A Glance

PattyGraph's time grid uses 60 log-time seconds per interval, with a maximum
retained history depth of 80 intervals.

The core controls are:

```text
push   higher value means stronger aging pressure and shorter purge windows
scale  higher value makes repeated words and refs become Peak more readily
grace  higher value requires more intervals before Peak eligibility
flux   higher value ranks entries with a deeper recency bias
```

The related display markers are:

```text
Peak   pinned survivor that remains until purged
+      entry has full retained history for the session or 80-interval window
```

## A Garbage-Collector Shape

The mental model is close to garbage collection applied to display state.

Interesting entries are constantly being tested by time:

```text
new entry appears
entry is seen again and survives
entry builds history
entry stops being seen and ages out
entry survives long enough and can become Peak
```

Like a garbage collector, PattyGraph is trying to keep the active working set
small enough to be useful. A token, referer, or IP that appeared once should not
hold screen space forever. A key that keeps surviving across intervals earns
more attention.

This is display-state pressure on the TUI's live working set.

## A Sliding-Window Grep Shape

Another way to think about it is a bulk grep with a sliding window.

A normal grep answers:

```text
did this pattern appear anywhere in the file?
```

PattyGraph is closer to asking:

```text
is this pattern still active enough inside the moving window to stay visible?
```

That difference matters during live traffic. A one-time match has limited value
once the traffic moves on. A token that keeps matching as the window moves is
more likely to describe the current traffic shape.

Time pressure is what turns matching into a live view:

```text
grep finds the key
the window keeps testing it
push removes it if it goes quiet
history makes repeated activity visible
Peak preserves durable findings
```

## The Interesting Columns

PattyGraph keeps three live interesting streams:

```text
words  request and User-Agent tokens
refs   referer tokens
ips    source IPs and prefix groups
```

Each entry has interval state:

- how many times it was seen in the current interval
- recent interval history
- the source line examples PattyGraph retained for it
- whether matcher coloring associated it with a known matcher
- for IPs, burstiness and User-Agent movement signals

At the end of an interval, PattyGraph pushes the current count into the entry's
history and resets the current count for the next interval. Entries that stop
being seen are allowed to age out.

## Push

`--push` controls how aggressively idle entries are removed from the interesting
columns.

Higher push means stronger time pressure. Lower push means entries are allowed
to remain visible longer after their last hit.

Push goes to 11 because sometimes maximum pressure is not enough.

The push setting maps to different retention windows for each interesting
stream:

```text
push  words  refs  ips
0     300    300   300
1     240    300   300
2     180    240   300
3     120    180   240
4      90    120   180
5      60     90   120
6      30     60    90
7      20     30    60
8      10     20    30
9       5     10    20
10      1      5    10
11      1      1     1
```

These values are log-time seconds, ranging from `1` to `300`. They are the purge
windows shown in expert mode. In the normal live view, the practical meaning is:

```text
when an entry goes unseen beyond its window, it can be purged
```

Among the interesting columns, `words` usually has the shortest window because
request and User-Agent tokens can be broad and numerous. `ips` usually has the
longest window because source behavior often needs more time to become meaningful.

## Scale

`--scale` changes how strongly frequency contributes to whether a non-IP entry
is interesting enough to become Peak.

For `words` and `refs`, PattyGraph computes a normalized score from the entry's
retained history and compares it against the stream's traffic baseline. Raising
scale makes repeated tokens more likely to cross that threshold. Lowering scale
makes the Peak threshold harder to reach.

Scale changes promotion pressure. Repeated activity crosses into Peak more
readily at higher scale and stays more selective at lower scale.

## Grace

`--grace` controls how long an entry must survive before it can become Peak.

An entry can be noisy in one interval and still disappear. Grace prevents that
single burst from becoming permanent too quickly. PattyGraph waits until enough
intervals have passed and the entry has enough retained history before it can
qualify for Peak.

Conceptually:

```text
seen once       maybe interesting right now
seen repeatedly allowed to build history
survives grace  eligible for Peak
```

The default retained history depth is `80` intervals. Grace is the gate before
an entry can be promoted into the persistent Peak area.

## Peak Entries

Peak entries are the items PattyGraph has decided should stay visible even when
ordinary time pressure would otherwise remove them.

In the TUI, Peak entries are visually separated and colored differently from the
normal changing list. They are pinned at the top of their respective interesting
column.

Peak means:

```text
this key has survived long enough and scored strongly enough to deserve a stable slot
```

That stable slot is useful for ordinary site shape as well as threat shape.

For `words`, Peak often identifies important parts of the site. If a site is
serving many `/catalog/...` requests, `catalog` can keep scoring high and become
Peak because it describes a durable part of the traffic.

For `refs`, Peak can show the referers or content paths that repeatedly explain
where traffic is coming from.

For `ips`, Peak usually deserves closer interpretation. A Peak IP can be
expected traffic, such as continuous crawler attention from a known service, or
it can be a source that keeps returning through reuse, rotation, or sustained
watching. The value is that it remains visible long enough to compare against
matchers, prefix groups, errors, bytes, and User-Agent movement.

Peak entries remain pinned until an explicit purge action clears them, such as
the keyboard purge command or the inline command:

```text
!!! purge
```

## PrimeFlux And Recent Pressure

PattyGraph ranks interesting entries using the current interval plus recent
interval pressure.

`primeFlux` is the current interval plus recent retained history. The depth of
that recent history is controlled by `flux`.

Lower flux makes the list more reactive. Higher flux makes the list steadier
because more recent intervals are folded into the score.

Think of `flux` as a prescribed depth of recency bias. PattyGraph asks how much
pressure a key has when the current interval is read with the last few
intervals.

For example, with `flux` set to `3`:

```text
entry      current  previous intervals      primeFlux
admin      8        7, 6, 4                 25
login      9        1, 0, 0                 10
cart       4        5, 5, 5                 19
```

`login` has the highest current count, but `admin` has stronger recent pressure.
That makes `admin` rank as more interesting because it has been applying
pressure across the recency window.

With a lower flux depth, the display reacts more sharply to the current
interval. With a higher flux depth, the display is more biased toward entries
that keep showing up across several intervals.

The useful mental model:

```text
count      what is happening this interval
flux       how much recent history to bias toward
history    what has been happening recently
primeFlux  current pressure plus flux-depth recent pressure
```

## The `+` Marker

An interesting entry can show a `+` at the front of its row.

That marker means the entry has full retained history for the current session:
either it has been present for every completed interval so far, or it has filled
the full `80` interval history window.

Example:

```text
+example     1.23
 example     0.42
```

The `+` marks history depth. Peak uses separate placement and color. The marker
tells the operator that PattyGraph has a complete retained history window for
that key, which makes its secondary metrics and mini sparkline more meaningful.

## Expert Overlay

Expert mode shows the time-pressure controls directly in the top overlay.

An example from the help text:

```text
20 3000 80M@/{4:90.120.180}301.5
```

The time-pressure portion is:

```text
{4:90.120.180}301.5
```

Read it as:

```text
4            push factor
90.120.180   purge windows for words, refs, ips
30           grace
1.5          scale
```

That overlay is a compact way to see how hard PattyGraph is pushing stale
entries out of the interesting columns.

## Diagnosing Time Pressure With Factoids

Some factoids expose the same pressure from another angle.

The `interesting.oneHitRefs`, `interesting.oneHitWords`, and
`interesting.oneHitIPs` factoids display as `1-Hit Refs`, `1-Hit Words`, and
`1-Hit IPs`. They show how much of each interesting stream is barely surviving
in the current recency window by counting keys whose `primeFlux` is `1`.

Example:

```text
1-Hit Refs:42(68%)
```

Read that as:

```text
42 tracked referer keys have only one unit of recent pressure
those keys are 68% of the current refs working set
```

That pattern suggests broad, shallow activity. Many keys are present, but most
of them are barely repeating inside the current `flux` window. Under stronger
`push`, many of those shallow keys may age out quickly. Under weaker `push`,
more of them may remain visible long enough to build history.

These factoids can help explain why the interesting columns feel busy, thin, or
unstable under a given `push` setting.

## Tuning By Traffic Shape

Every site has its own useful pressure range.

The right pressure depends on:

- total request volume
- how broad the request vocabulary is
- how many unique referers appear
- whether source IPs repeat or spread out
- whether traffic is bursty or steady
- whether the operator wants fast reaction or stable context

Lower pressure is useful when traffic is quieter and you want to keep more
context on screen.

Higher pressure is useful when traffic is broad or noisy and stale entries make
the columns harder to read.

More scale can help repeated entries become Peak sooner. Less scale keeps Peak
more selective.

More grace makes PattyGraph require longer survival before promotion. Less
grace lets persistent entries become Peak earlier.

## Why This Matters

Without time pressure, PattyGraph would become a growing list of old keys.

With time pressure, the interesting columns stay live:

```text
new keys can enter
idle keys can leave
repeated keys can build history
durable keys can become Peak
```

That is why the TUI can stay useful during a live incident. PattyGraph is
constantly deciding which tokens, referers, and sources still deserve screen
space.

## Short Version

Time pressure is the aging system behind the interesting columns. `push`
controls how quickly idle entries are purged. `scale` changes how strongly
repeated activity contributes to Peak promotion. `grace` controls how long an
entry must survive before Peak is possible. Peak entries stay pinned until
purged, and a leading `+` means the entry has full retained history.
