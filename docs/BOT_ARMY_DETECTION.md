# Bot And Bot-Army Detection

PattyGraph treats bot detection as a live traffic-shape problem.

The goal is not to prove identity from a single User-Agent string. The goal is
to notice when bot-like traffic becomes operationally important, then expose the
source shape: which bot names are showing up, which IPs or prefixes are
participating, which requests are being hit, and whether errors or bytes are
rising with the traffic.

## Bot-Like Traffic

The built-in Bots matcher is PattyGraph’s broad bot-like traffic detector. It
looks for common bot-like words in User-Agent text:

```text
bot
spider
crawler
agent
```

When a request looks bot-like, it contributes to the `Bots` row. The `Bots`
breakdown in the lower-left pane shows which bot words are inside the aggregate.

```text
Bots row:        how much bot-looking traffic is happening
Bots breakdown: which bot names are inside that traffic
```

This is intentionally broad. A high `Bots` row means "bot-like User-Agent
traffic is active enough to watch," not "all of this traffic is malicious."

## Promoted Bot Rows

When a specific bot name dominates the `Bots` aggregate, PattyGraph can split it
out into its own matcher row above `Bots`.

Before promotion:

```text
Bots       900  ...  sparkline
```

After promotion:

```text
Googlebot  640  ...  sparkline
Bots       260  ...  sparkline
```

That makes the screen easier to read. The promoted bot gets its own count,
trend marker, previous value, sparkline, and breakdown. The remaining `Bots`
row still tracks other bot-like traffic that has not been split out.

This is useful when a single bot is large enough to flatten the meaning of the
aggregate view.

## Bot Armies

In this context, “bot army” means automated traffic whose distributed sources
or coordinated behavior form a visible operational pattern. A bot army is not
just "many bot requests." In PattyGraph terms, it usually looks like bot-like
or automated traffic distributed across a source shape:

- many IPs sharing a prefix
- many prefixes sending similar requests
- one User-Agent family spread across many IPs
- many User-Agent strings hitting the same URL shape
- elevated line volume with matching error or byte patterns
- IPs or prefixes that appear abruptly and move together

PattyGraph does not need to name the campaign perfectly. It tries to make the
shape visible while it is happening.

## Where To Look In The TUI

Start with the matcher rows:

- `Bots`: broad bot-like traffic
- promoted bot rows: specific bot names split out from `Bots`
- IP rows: a single IP important enough to deserve its own lane
- `lines`: total request pressure
- `bytes`: response volume
- `errs`: failures rising with the traffic

Then use the lower panes:

- matcher breakdowns show bot names, error codes, or prefix/source shape
- `words` shows request and User-Agent tokens
- `refs` shows referer tokens
- `ips` shows active IPs and prefix groups

The bot rows answer:

```text
Which bot identities are appearing in the traffic?
```

The interesting panes answer:

```text
What is that traffic doing, and where is it coming from?
```

## IP Prefix Groups

The `ips` column is the main place to look for bot-army source shape.

A shared prefix is a clustering signal, not proof that the addresses have the
same owner or are part of one coordinated system. Gateways and recycled IP's
can also be found in this space. 

PattyGraph tracks individual IPs, but it also groups active IPs by prefix. A
prefix row can reveal that several related IPs are moving together even if no
single IP is the whole story. 

Conceptually:

```text
203.0.*.*(18)     many active IPs in one source neighborhood
198.51.*.*(7)     smaller but still grouped source activity
```

Prefix grouping is especially useful when a bot army is spread just enough that
individual IP rows look less important than the group.

## User-Agent Movement

Tab mode `3` shows averaged User-Agent delta. For IPs and prefix groups, this
can help identify traffic where the source is not just active, but changing its
User-Agent shape.

High User-Agent movement can suggest:

- rotating bot strings
- mixed tooling behind one IP or prefix
- spoofed browser families
- shared infrastructure producing inconsistent clients

It does not automatically mean "bad bot." It means the retained User-Agent
shape is moving enough to investigate.

## Common Screen Patterns

### `Bots` High, Prefix Groups Low

Bot-like traffic is active, but it may be spread across many unrelated sources.
Look at the `Bots` breakdown to see which bot words are driving the aggregate.

### `Bots` High, One Prefix Group High

This is a stronger bot-army shape. A broad bot-like signal and a source
neighborhood are visible at the same time.

### Promoted Bot High, `Bots` Low

One known bot explains most of the bot-like traffic. Watch the promoted row's
sparkline and inspect its source breakdown.

### Promoted Bot High, `Bots` Also High

One known bot is active, but it does not explain all bot-like traffic. Read the
promoted row and the catch-all `Bots` row together.

### `errs` Rises With Bot Or Prefix Activity

The bot traffic may be causing failures or probing error-prone paths. Inspect
`errs` for response codes, then select request words or IP groups for source
context.

### `bytes` Rises Without Matching `lines`

The bot traffic may be moving heavier responses rather than making many small
requests. Use the bytes row and Tab bytes mode to find which keys are carrying
the weight.

## Useful Actions

During a live session:

1. Watch `Bots`, promoted bot rows, `lines`, `bytes`, and `errs`.
2. Use the matcher breakdown column to see bot names or source prefixes.
3. Check `ips` for grouped source activity.
4. Select the suspicious IP or prefix.
5. Use `Tab` to cycle burstiness, flux, history depth, User-Agent delta,
   mini-sparkline, and bytes.
6. Add a matcher or alert when a pattern is worth tracking directly.

Examples:

```text
!!! alert Bots above 500
!!! alert errs above 50
!!! select --ips 203.0.
```

## What PattyGraph Does Not Claim

PattyGraph does not prove intent. It does not verify that a User-Agent claiming
to be a known bot is authentic. It does not replace firewall logs, WAF data,
reverse DNS verification, or raw-log forensic review.

It gives an operator fast evidence about live access-log shape:

- bot-like User-Agent traffic
- dominant bot names
- distributed source patterns
- request and referer texture
- errors, bytes, and history
- selected source-line examples

That is usually enough to decide what deserves deeper investigation.

## Relationship To PattyLog

When PattyLog JSONL is enabled, the same signals are available outside the TUI:

- matcher counts and histories
- top matcher keys
- matcher prefix groups
- `words`, `refs`, and `ips` top entries
- IP groups
- selected source-line context
- alert transitions

That lets a human, script, or AI-assisted workflow inspect the bot-army shape
without ingesting the full raw log stream first.

## Short Version

`Bots` shows bot-like User-Agent traffic. Promoted bot rows split dominant bot
names into their own lanes. The `ips` column and prefix groups show whether the
traffic is concentrated, distributed, or moving like a bot army. PattyGraph does
not prove intent; it makes the live shape visible fast enough to act on.
