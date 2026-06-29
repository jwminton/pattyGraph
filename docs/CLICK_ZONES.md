# Click Zones

PattyGraph's terminal UI is clickable, but it is not built like a normal widget
tree. The screen is rendered as dense text panes, and mouse clicks are
interpreted by where they land on that rendered surface.

In practice, that means most of the TUI behaves like a set of click zones.
There are only a few places where a click is intentionally a no-op. The goal is
that a human can click what looks interesting and get a useful response without
thinking about the underlying terminal layout.

## Screen Regions

PattyGraph has two main vertical regions:

```text
sparkline / history area
--------------------------------
matcher breakdown | words | refs | ips
```

The top region shows matcher rows, history sparklines, ticker/status content,
and selected-line context.

The bottom region has four columns:

- matcher breakdowns
- `words`
- `refs`
- `ips`

Each region has its own click behavior.

## Quick Interaction Map

| Action | Result |
| --- | --- |
| Click matcher row | Select matcher |
| Click matcher sparkline | Inspect retained interval value |
| `Ctrl` + click matcher | Cycle matcher detail level |
| Click `words`, `refs`, or `ips` | Select interesting item |
| Click selected-item sparkline | Inspect selected item's retained history |
| Click ticker | Toggle factoid display mode |

## Sparkline Area

The sparkline area is the main matcher-selection and history-inspection zone.

Clicking a matcher row selects that matcher. A selected matcher is highlighted
and can be used by keyboard commands such as moving, deleting, purging, or
creating follow-up matchers.

Clicking the sparkline portion of a selected matcher row also selects a specific
history interval value. PattyGraph displays that clicked value at the top of the
screen in a compact form appropriate to the matcher:

- `bytes` values are formatted as bytes.
- `lines` values are formatted as counts.
- large matcher values may be trimmed for display.

That makes the sparkline more than a picture. It is a compact history browser.

```text
matcher name / count | sparkline history
click row            | click interval value
```

## Interesting Sparkline

When a `words`, `refs`, or `ips` entry is selected, PattyGraph adds a selected
interesting-item sparkline below the matcher rows.

Clicking that sparkline selects a history value for the selected interesting
key. The selected value is shown at the top of the screen to the left of the running log time and can also be sent to the ticker/fact stream when the ticker is visible.

The selected interesting sparkline also carries source-line context. PattyGraph
prints a representative log line beneath the selected sparkline so the selected
key is tied back to an actual request.

That log line follows the same `Tab` cycle that changes the secondary values in
the `words`, `refs`, and `ips` lists. Tabbing through the secondary views changes
which stored source example is emphasized under the sparkline. The source-line
context cycles through three representations:

- first source line for the key
- first source line for the key in the current interval
- last seen source line for the key

The selected key does not change when `Tab` is pressed. The selected sparkline
stays focused on the same `words`, `refs`, or `ips` entry while the secondary
view and source-line lens change together.

This is useful when an interesting key has a visible spike and you want to know
the value behind a specific point in its retained history, and then inspect the
source request that explains the selected traffic shape.

## Matcher Breakdown Column

The lower-left column expands matcher details. It is still part of matcher
selection, but it operates on the expanded matcher view instead of the compact
sparkline row.

Clicking inside a matcher's expanded block selects that matcher.

For example:

```text
Bots(4)      128
 googlebot    80
 bingbot      31
 crawler      17
```

Clicking anywhere in that `Bots` block selects `Bots`.

For a promoted bot or IP-like matcher, the expanded block may show prefix or
source-shape details. Clicking that block selects the matcher that owns those
details.

## Expanding Matcher Detail

`Ctrl` + click on a matcher selection zone cycles that matcher's detail mode.

This works in the matcher row area and in the lower matcher breakdown column.
It changes how much match detail is printed for that matcher:

```text
minimal -> normal -> expanded -> minimal
```

The matcher breakdown title includes a small expansion marker:

```text
no marker   minimal
·           normal detail
:           expanded detail
```

So a matcher title with no marker is at the most compact level, a title with
one dot is showing normal detail, and a title with `:` is showing the expanded
view.

This is useful when a matcher has many stale or low-count entries and you want
to reveal or hide more of its breakdown without changing what PattyGraph is
collecting.

## Interesting Columns

The `words`, `refs`, and `ips` columns are text views that behave like selectable
lists.

Clicking an entry in one of those columns selects that interesting key:

- click `words` to select a request/User-Agent token
- click `refs` to select a referer token
- click `ips` to select an IP or prefix group

Selection exposes context PattyGraph already has for that key:

- highlight state in the column
- selected-key sparkline in the top area
- source-line context below the graph
- selected context in PattyLog JSONL when enabled

The columns are not traditional list widgets, but they are meant to feel like
lists to the user.

## Ticker Click

When the ticker is visible, clicking its line in the top region toggles the
ticker display mode.

In normal mode, the lower-left pane shows matcher breakdowns. Clicking the
ticker switches that lower-left pane into a factoid display list. In this mode,
the pane is informational rather than an interactive matcher-selection area, and
it is headed by the current PattyGraph name/version display.

The ticker background switches to blue while this factoid display mode is
active. Clicking the ticker area again toggles back to normal matcher-breakdown
mode.

This is a small control surface compared with matcher and interesting-item
selection, but it follows the same spatial model: click the rendered thing you
want to affect.

## No-Op Areas

Some clicks intentionally do nothing. Common no-op cases include:

- clicking outside the rendered PattyGraph width
- clicking above the active graph content
- clicking blank space that does not map to a matcher, breakdown, or interesting
  entry
- clicking the matcher breakdown column while the metrics/history panel is
  replacing normal matcher breakdowns

These no-op areas are usually small. Most visible operational content has a
click response.

## Selection Types

PattyGraph has two broad click-driven selection types.

Matcher selection:

```text
click matcher row
click matcher breakdown block
```

Interesting selection:

```text
click words entry
click refs entry
click ips entry
```

They answer different questions.

Matcher selection asks:

```text
Which traffic lane or matcher am I operating on?
```

Interesting selection asks:

```text
Which word, referer, IP, or prefix am I investigating?
```

## Why It Works This Way

PattyGraph is intentionally TUI-first. It does not try to turn every part of the
terminal into a formal widget. The screen is a dense operational surface, and
the click model follows the visual layout.

That lets PattyGraph keep the display compact while still supporting direct
interaction:

- select a matcher by clicking its row
- inspect a graph value by clicking a sparkline
- expand matcher details with `Ctrl` + click
- select an interesting key by clicking its column entry

The result can be hard to describe in abstract terms, but it is meant to feel
natural while watching the live screen: click the traffic shape or entry that
catches your attention, and PattyGraph focuses that part of the session.

## Relationship To PattyLog

Clicking changes TUI selection state. When PattyLog JSONL is enabled, selected
context may appear in interval records.

For interesting selections, PattyLog can expose:

- selected matcher name for `words`, `refs`, or `ips`
- selected key
- selected graph value
- source-line context

For matcher selections, PattyLog can expose the selected matcher name.

That makes click-driven investigation visible outside the terminal without
turning the terminal UI into a separate data source.
