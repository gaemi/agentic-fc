# Agent Alert Design

Agent Alerts are the bridge between Agentic FC's real-time world and long-running
AI agent harnesses. They let an Agent declare which game events are worth waking
for, and let an MCP host or harness receive a standard MCP resource update when
one of those conditions becomes true.

Alerts do **not** make the daemon run an Agent. The daemon only exposes a
manager-scoped signal. The Agent host decides whether a notification starts a
new reasoning loop, waits for more events, or ignores it.

## 1. Goals

- Reduce blind polling without requiring the simulation to wait for Agents.
- Preserve the existing gameplay boundary: MCP remains the Agent play surface,
  Console API remains the spectator/operator surface.
- Keep hidden attributes, exact formulas, and spectator-only feed details out of
  Agent notifications.
- Keep replay deterministic: watch configuration and alert acknowledgements are
  accepted MCP inputs; world outcomes do not depend on whether a client is
  connected to the notification stream.
- Support common long-running harness patterns: subscribe, wait, wake, inspect
  with normal tools, then shape if needed.

## 2. Non-goals

- No daemon-managed LLM calls, scheduling, prompt execution, or agent hosting.
- No direct webhook integrations in the core daemon for v1. A webhook bridge can
  be an external MCP client/harness that subscribes to alerts.
- No free detailed scouting, match, or squad information in alert payloads.
  Alerts are pointers and triage hints; normal tools provide detail at their
  normal Focus costs.
- No use of the Console SSE feed as the Agent alert stream. Console feed events
  are spectator-facing and may contain richer public presentation than MCP
  should expose to an Agent.

## 3. Protocol Shape

The MCP server exposes one manager-private resource:

```text
agenticfc://manager/self/alerts
```

The URI is resolved from the authenticated Manager Token. Clients do not pass a
manager id in the URI, so a token cannot request another Manager's alert queue.

Clients that support MCP resource subscriptions can call `resources/subscribe`
for that URI. The resource is also readable for host compatibility.
`resources/read` returns only static guidance with MIME type `application/json`:

```json
{
  "resource": "agenticfc://manager/self/alerts",
  "hint": "Call get_alerts for pending alert summaries; call ack_alerts after handling them."
}
```

Reading the resource never acknowledges alerts, never returns pending counts or
cursors, and never returns football detail. It exists for subscription bootstrap
and host integration. State discovery goes through the Focus-priced
`get_alerts` tool.

When a subscribed Manager has new pending alerts, the server sends the standard
MCP resource update notification:

```json
{
  "jsonrpc": "2.0",
  "method": "notifications/resources/updated",
  "params": {"uri": "agenticfc://manager/self/alerts"}
}
```

The notification is only a wake signal. The client calls `get_alerts` to
retrieve the current pending alert list. If a client does not support resource
subscriptions, it can still poll `get_alerts`, but that polling is Focus-priced.
`configure_alerts` and `ack_alerts` do not emit resource update notifications;
only newly issued pending alerts do.

## 4. MCP Tools

The alert surface adds three tools. Watch configuration and acknowledgement are
free because they manage the transport contract. Reading pending alert summaries
costs the same as `get_news`, so a client that falls back to polling still pays
for repeated attention. Detailed follow-up still goes through Focus-priced tools
such as `get_news`, `get_match`, `get_person`, or `get_squad`.

### `configure_alerts` - 0 FP

Replaces the authenticated Manager's watch configuration.

Params:

```json
{
  "enabled": true,
  "watches": [
    {"kind": "NEWS", "categories": ["board", "injury"], "scope": "own"},
    {"kind": "MATCH", "when": "OWN_KICKOFF", "lead_minutes": 120},
    {"kind": "FOCUS", "threshold": 25, "edge": "rising"},
    {"kind": "CALENDAR", "when": "DATE_CHANGED"}
  ]
}
```

Rules:

- `enabled=false` disables delivery but preserves the configured watches.
- While disabled, watches do not create alerts and do not emit notifications.
  Re-enabling resumes future evaluation only; missed crossings are not backfilled.
- A Manager may have at most 32 watches.
- Replacing watches does not clear already pending alerts.
- `FOCUS.threshold` must be an integer from 0 through the Focus cap, inclusive.
  Out-of-range thresholds fail with `VALIDATION`.
- Invalid enum values fail with `VALIDATION`.

### `get_alerts` - 1 FP

Returns the current watch configuration, pending alerts, the alert resource URI,
and client guidance.

Params:

```json
{"limit": 50}
```

Result shape:

```json
{
  "resource": "agenticfc://manager/self/alerts",
  "enabled": true,
  "watches": [],
  "pending": [
    {
      "id": 1204,
      "game_time": "1926-03-07T14:30",
      "kind": "NEWS",
      "reason": "board",
      "refs": [{"news": 441}],
      "message": "Board warning filed. Use get_news with scope=own for detail."
    }
  ],
  "highest_issued_id": 1204,
  "acked_through": 1198,
  "next_cursor": "1204",
  "subscribe_hint": "Subscribe to agenticfc://manager/self/alerts and call get_alerts when notifications/resources/updated arrives."
}
```

`get_alerts` does not acknowledge alerts. It is safe to call repeatedly after a
wake notification.

### `ack_alerts` - 0 FP

Acknowledges delivered alerts so they no longer appear in `pending`.

Params:

```json
{"through": 1204}
```

`through` is inclusive and numeric. Alert ids are monotonic and contiguous per
Manager. Validation rules:

- `through > highest_issued_id` fails with `VALIDATION`; future alerts cannot be
  swallowed.
- `through <= acked_through` is a no-op and returns the unchanged cursor.
- `acked_through < through <= highest_issued_id` advances the Manager-wide
  acknowledgement cursor and returns the applied value.

Acknowledgement is logged as an accepted MCP input, making the pending-alert
cursor deterministic under replay.

## 5. Watch Kinds

Initial watch kinds:

| Kind | Trigger | Public detail in alert |
|------|---------|------------------------|
| `NEWS` | A visible news item matching category/scope/filter appears. | News id, category, refs, generic prompt to call `get_news`. |
| `MATCH` | Own kickoff lead time, own kickoff, own full time, or selected fixture state. | Fixture/match refs and time only. |
| `CALENDAR` | Date changed, transfer window opened/closed, season ended. | Calendar marker and game time. |
| `FOCUS` | Focus balance crosses a configured threshold. `edge` is `rising` for "enough Focus to act" or `falling` for "low Focus warning"; default is `rising`. | Threshold, edge, and current balance. |

Future watch kinds may include player-, club-, or directive-specific filters,
but they must still return only information the Manager could learn through the
normal MCP surface.

Focus watches are edge-triggered, not level-triggered. They use a 2 FP
hysteresis band:

- `rising` arms only when the balance is at or below `threshold - 2`, then fires
  when the balance reaches or crosses `threshold`.
- `falling` arms only when the balance is at or above `threshold + 2`, then
  fires when the balance reaches or crosses `threshold`.
- After firing, a Focus watch is disarmed until the emitted alert is
  acknowledged and the balance has returned to the arming side of the hysteresis
  band.

At most 128 pending alerts per Manager may be `FOCUS` alerts. This sub-quota
prevents Focus oscillation from evicting match, calendar, or news wake signals.
When a new Focus alert would exceed the sub-quota, the oldest pending Focus
alert is evicted and the new Focus alert is inserted; non-Focus alerts are never
evicted to make room for Focus alerts unless the global cap itself is full.

## 6. Visibility and Privacy

Alert visibility follows `get_news` and the existing observation tools:

- Manager-private news wakes only the owning Manager.
- `scope=own` news wakes only when the item is visible to the Manager's own
  context.
- `scope=league` and `scope=world` use the same breadth rules as `get_news`.
- Alert payloads never include hidden attributes, exact private formulas,
  scout-derived private numbers, or spectator-only feed values.
- An unemployed Manager keeps public alert watches, but own-club internal
  watches become inert until the Manager is employed again.

## 7. Capacity, Persistence, and Replay

Watch configuration, pending alerts, and the acknowledgement cursor are part of
the world snapshot. They are Manager state, like Focus and Mindset.

Each Manager keeps at most 512 pending alert records. When a new alert would
exceed the cap, the oldest pending records are evicted and one synthetic
`SYSTEM` alert with reason `OVERFLOW` is inserted. That pending overflow alert
is protected from eviction. While it is still pending, further cap pressure
evicts the oldest non-overflow records without inserting additional overflow
alerts. Overflow insertion rearms only after the harness acknowledges below the
overflow alert id. The overflow alert tells the Agent to catch up through broad
normal observation (`get_situation`, `get_news`, and league/match reads) because
some wake signals were dropped.

Accepted calls to `configure_alerts`, `get_alerts`, and `ack_alerts` enter the
input log. `get_alerts` costs 1 FP, limiting blind polling through the existing
Focus economy. `resources/read` of the alert URI is not a gameplay tool, returns
only static subscription guidance, and is not a substitute for `get_alerts`.

Resource update delivery itself is not replay-authoritative. If a client was
offline, pending alerts remain in the snapshot and are returned on the next
`get_alerts` call.

Alert acknowledgement is Manager-wide, matching the existing rule that
concurrent sessions on one Manager Token share state. A supervisor or debugging
session may read alerts, but only the primary long-running harness should call
`ack_alerts`; otherwise it can intentionally acknowledge alerts for every
session on that token. This is no different from shared Focus and last-write-wins
Mindset edits: concurrent sessions require harness coordination.

## 8. Engine Integration

The Simulation Core remains single-writer. Alert creation is derived from
already-committed world state:

- News alerts derive from `World.News` items after they are appended.
- Match and calendar alerts derive from committed fixture/calendar events.
- Focus alerts are emitted by deterministic Manager alert events in the
  Simulation Core's discrete-event queue. `configure_alerts` and Focus-spending
  MCP calls schedule or supersede future alert events at the exact game time
  where constant Focus regen will cross a watch threshold. When such an event
  drains, the engine rechecks the watch version, current balance, edge, and
  hysteresis before appending the alert. Stale events are no-ops. This means the
  alert id and `game_time` come from queue order, not wall-clock timer jitter.

The notifier that emits MCP `notifications/resources/updated` is an observer of
new pending-alert issuance. Slow or disconnected clients never block the engine.

## 9. Agent Harness Loop

A capable long-running harness can use this loop:

1. Authenticate with a Manager Token.
2. Call `get_guide`, then `configure_alerts`.
3. Subscribe to `agenticfc://manager/self/alerts`.
4. Sleep until `notifications/resources/updated` arrives.
5. Call `get_alerts`, then normal observation tools such as `get_news`,
   `get_situation`, or `get_match`.
6. Shape Mindset/Tactical Plan if needed.
7. Call `ack_alerts` after it has handled the pending alerts.

If subscription support is missing, the same harness can poll `get_alerts` on
its own schedule. Polling remains a fallback, not the only designed path.
