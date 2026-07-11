# Operations Guide

This guide covers local development and ordinary operation of Agentic FC.

## Build

```sh
make build
```

This builds all packages and writes local binaries to `bin/`:

- `bin/agenticfc`
- `bin/agenticfc-console`
- `bin/agenticfc-calibrate`

You can also run Go commands directly:

```sh
go test ./...
go vet ./...
go build ./...
```

## Start a World

```sh
./bin/agenticfc \
  -data ./data \
  -preset compact \
  -profile fast \
  -seed 42 \
  -world-name "Codex Fastest Run" \
  -club-names "Agentic FC,Codex United" \
  -manager-names-file ./manager-names.txt \
  -start
```

Useful daemon flags:

| Flag | Default | Meaning |
|------|---------|---------|
| `-data` | auto | Data directory for snapshot, manifest, logs, and tokens. Omitted: `./data` if it already holds a world, else the OS user data directory (see below). |
| `-console-addr` | `127.0.0.1:7420` | Console API listen address. |
| `-mcp-addr` | `127.0.0.1:7421` | MCP Streamable HTTP listen address. |
| `-preset` | `classic` | New-world preset: `compact`, `classic`, `deep`, `sprawling`. |
| `-seed` | `0` | New-world seed; `0` means random. |
| `-world-name` | generated | Display name for a new world. |
| `-club-names` | none | CSV list of custom club names, applied from the first generated club onward. |
| `-club-names-file` | none | Line-separated custom club names appended after `-club-names`; blank lines and `#` comments ignored. |
| `-manager-names` | none | CSV list of custom manager names, applied to club managers first, then unemployed managers. |
| `-manager-names-file` | none | Line-separated custom manager names appended after `-manager-names`; blank lines and `#` comments ignored. |
| `-profile` | `default` | Run profile: `default`, `fast`, `slow`, `custom`. |
| `-speed` | profile value | Match-window speed override: `5`, `15`, `30`, or `60`. |
| `-idle-accel` | profile value | In-season idle acceleration multiplier. |
| `-offseason-accel` | profile value | Off-season acceleration multiplier. |
| `-start` | `false` | Start simulation immediately. |
| `-snapshot-interval` | `1m` | Periodic snapshot cadence. |
| `-widget-mode` | `apps` | MCP UI mode: `apps`, `meta`, or `content`. |
| `-widget-locale` | `""` | MCP UI locale override: supported language tag resolving to `en` or `ko` (for example `ko-KR`); empty follows client/system language. |

Invalid `-widget-locale` values fail startup so deployments do not silently pin
the UI to an unintended language.

Generation flags apply only when no world exists in the data directory. A
subsequent daemon run resumes the existing world and ignores these creation
flags.

### Listen Addresses and Port Conflicts

The daemon binds both listen addresses before it reads or writes the data
directory. If either address is unavailable — most commonly because another
`agenticfc` daemon is already running on the default ports — the launch fails
immediately with a hint naming the flag to change, and no world data is
created or consumed by the failed launch.

To run several daemons side by side, give each one its own data directory and
its own ports:

```sh
./bin/agenticfc -data ./data-b -console-addr 127.0.0.1:7430 -mcp-addr 127.0.0.1:7431 -start
```

Port `0` asks the OS for a random free port. The startup banner always prints
the addresses that were actually bound (wildcard hosts are shown as the
matching loopback address — `127.0.0.1` or `::1` — so the URL is directly
dialable), so `-console-addr 127.0.0.1:0` is a safe way to launch without
picking a port first; point the console's `-server` flag at the printed
address.

Because the ports are bound before the world loads, a client that connects
during startup receives `503 Service Unavailable` with a `Retry-After` header
until the daemon is ready, rather than a connection refusal or a stalled
request.

### Ready Worlds

A new world created without `-start` is **ready**: fully generated and
observable, but with the game clock stopped (Focus does not regenerate
either). The daemon prints how to start it. Either relaunch with `-start`, or
start it in place through the Console API:

```sh
curl -X POST http://127.0.0.1:7420/v1/admin/start \
  -H "Authorization: Bearer $(cat ./data/admin.token)"
```

Custom name rules:

- A custom name list may provide only the first few names; the rest of the clubs
  or managers keep generated names.
- Empty names, duplicate names within the same list, names longer than 64
  characters, names containing tabs or newlines, or lists longer than the
  generated entity count fail world creation before anything is written.
- Club short names are derived automatically from custom club names after common
  football tokens such as `FC`, `AFC`, and `United` are ignored. The current
  short-name derivation keeps only ASCII `A`-`Z` letters and pads short sources.
- In name files, a `#` as the first non-whitespace character marks a comment
  line rather than a literal name.

## Data Directory

When `-data` is omitted, the daemon resolves the directory in this order:

1. `./data`, if that directory in the current working directory already holds
   Agentic FC world state (`world.json`, `manifest.json`, or `admin.token`).
   This keeps source checkouts and pre-existing local worlds working
   unchanged, while an unrelated project's `data/` folder is never adopted.
2. Otherwise, the per-user OS data directory:

   | OS | Default data directory |
   |----|------------------------|
   | macOS | `~/Library/Application Support/agenticfc` |
   | Linux / other | `$XDG_DATA_HOME/agenticfc`, default `~/.local/share/agenticfc` |
   | Windows | `%LocalAppData%\agenticfc` |

This is the natural layout for a packaged install (for example a future
Homebrew formula): the binary can be launched from any working directory and
always resumes the same world. The startup banner prints the resolved
directory as `data: …`. Pass `-data` explicitly to run several worlds side by
side or to pin a custom location.

The data directory contains local operational state:

| File | Purpose |
|------|---------|
| `snapshot.json` | Persisted world state and event queue. |
| `manifest.json` | Manager credentials and world metadata. |
| `admin.token` | Admin token for local operator control. |
| `audit.jsonl` | Roll audit trail. |
| `input.jsonl` | Accepted MCP input log. |

Do not commit real data directories. Tokens and snapshots may contain private
or unreleased world information.

## Open the TUI

```sh
./bin/agenticfc-console -server http://127.0.0.1:7420
```

Optional flags:

| Flag | Meaning |
|------|---------|
| `-server` | Console API base URL. |
| `-locale` | Display override: `en` or `ko`. |
| `-admin-token` | Enables Admin Mode screens and authenticated settings changes. |

Without `-admin-token`, the console stays in Viewer Mode and watches the world
through public spectator views. With `-admin-token`, press `5` for Settings and
use `+/-` or `[`/`]` to adjust runtime pacing live. The initial run-profile
flags still apply only when creating a new world, but Game Speed,
idle acceleration, and off-season acceleration can be changed after creation.

The Console API endpoints backing this screen are:

| Endpoint | Purpose |
|----------|---------|
| `GET /v1/admin/settings` | Read mutable runtime settings and their allowed ranges. |
| `PATCH /v1/admin/settings` | Partially update runtime pacing settings. Accepts one JSON object containing only `game_speed`, `idle_acceleration`, and/or `offseason_acceleration`; unknown fields or trailing JSON values return `400 Bad Request`. An empty object is a valid no-op. |

## Connect an MCP Client

The MCP gateway listens on the daemon's `-mcp-addr`. Use a Manager token from
`manifest.json` as the bearer token.

Example endpoint:

```text
http://127.0.0.1:7421
```

Recommended first tool calls for a fresh agent:

1. `get_guide`
2. `get_settings`
3. `get_time`
4. `get_situation`
5. `get_mindset`

Long-running agent harnesses can reduce blind polling by configuring Agent
Alerts after the first orientation pass:

1. Call `configure_alerts` with the news, match, calendar, and Focus conditions
   the harness wants to wake for.
2. Subscribe to the manager-specific MCP resource returned by `get_alerts` when
   the MCP host supports resource subscriptions.
3. On `notifications/resources/updated`, call `get_alerts`, then inspect detail
   through normal tools such as `get_news`, `get_situation`, or `get_match`.
4. Call `ack_alerts` after the harness has handled the pending alert ids.

MCP is the gameplay surface. It intentionally exposes public facts, scouting
uncertainty, Focus state, and controllable Mindset/Tactical Plan state, not raw
hidden traits or exact formulas.

## Run Calibration

`agenticfc-calibrate` generates compact worlds for seed batches, runs them for
a fixed game-time horizon, and emits deterministic aggregate match metrics.

```sh
./bin/agenticfc-calibrate -seeds 1,2,3,4,5 -days 365
```

The report includes match count, goals, shots, home/draw/away split, upsets,
chance type counts, shot quality, aerial volume, press turnovers, set-piece
threat, goals per match, shots per match, and conversion rate.

Use this for match-model tuning and regression checks. It is not required to
run a normal world.

## Verification

Before publishing or submitting a change:

```sh
go test ./...
go vet ./...
go build ./...
test -z "$(gofmt -l .)"
```

CI runs the same core checks.

## Versioning Policy

The root `VERSION` file is the source of truth for public release identity. It
contains a bare SemVer version with no `v` prefix, prerelease suffix, or build
metadata. Agentic FC starts at `0.1.0`.

Release tags add the `v` prefix. For `VERSION=0.1.0`, the release tag is
`v0.1.0`.

Release builds inject traceable build metadata into each binary's `--version`
output. The binary version therefore has the form
`v0.1.0+<commit_count>.g<short_sha>`, while the root `VERSION` file remains
`0.1.0`.

Before `1.0.0`, Agentic FC is still allowed to change public APIs, save data,
game balance, MCP tool shapes, and TUI presentation. Use the following rules:

- Bump **MINOR** for new user-facing gameplay systems, MCP/Console/TUI surface
  changes, save format changes, or any breaking pre-1.0 contract change.
- Bump **PATCH** for bug fixes, documentation updates, release tooling, and
  compatible polish that does not change public behavior or data contracts.
- Bump to **1.0.0** only when the project is ready to declare a stable public
  compatibility contract.

Every release-preparation change must update these files together:

- `VERSION`
- `CHANGELOG.md`, with a section exactly like `## 0.1.0 - YYYY-MM-DD`
- `docs/13-operations.md`, whose release tag and build metadata examples are
  pinned to the current `VERSION`

CI runs `make version-check`, which calls `scripts/check-version.sh`. The
version harness rejects malformed versions, missing changelog sections, stale
operation examples, and release workflows that do not read `VERSION` and run
`make verify` before packaging.

## Automated Draft Releases

The `draft-release` GitHub Actions workflow is a manual release-preparation
workflow. It builds packages from a selected ref and creates a draft GitHub
Release. It does not publish the release; an approver reviews the draft and
decides when to publish it.

Typical flow:

1. Merge the release-preparation changes, including any `VERSION` bump.
2. Let CI pass on the intended release ref, usually `main`.
3. Manually run the `draft-release` workflow from GitHub Actions. The workflow
   input `ref` selects the branch, tag, or commit SHA to package; the default is
   `main`.
4. The workflow runs `make verify` on that checkout.
5. The workflow reads the release version from the root `VERSION` file. For
   `VERSION=0.1.0`, the draft release tag is `v0.1.0`.
6. It cross-compiles all shipped commands and creates a GitHub Release marked as
   a draft.

The draft release packages include:

- `agenticfc`
- `agenticfc-console`
- `agenticfc-calibrate`
- `README.md`, `CHANGELOG.md`, and `LICENSE`

Each binary supports `--version`. Release builds inject the release tag and
commit SHA into that output, using the build metadata form
`v0.1.0+<commit_count>.g<short_sha>`, so bug reports can be traced back to the
published artifact.

Published target triples:

| OS | Architectures | Archive |
|----|---------------|---------|
| Linux | `amd64`, `arm64` | `.tar.gz` |
| macOS | `amd64`, `arm64` | `.tar.gz` |
| Windows | `amd64`, `arm64` | `.zip` |

The release also includes `checksums.txt` with SHA-256 hashes for every archive
and release notes linking back to the draft build run.

The workflow updates only an automation-owned draft release for the current
`VERSION`: if `v0.1.0` is still a draft with the workflow marker in its notes, a
later manual run can update that draft's target, notes, and packaged assets when
`replace_existing_draft=true`. It does not delete a published release. If
`v0.1.0` has already been published, the workflow fails instead of replacing it.
Bump `VERSION` before creating the next draft release.

Draft replacement is monotonic. A manual run for an older commit cannot rewind
the draft; the incoming commit must be the same as or a descendant of the
draft's current target.

The workflow marker is an HTML comment in the release notes. Removing that
marker, or using the same tag for a manually created draft, makes the workflow
fail rather than overwrite human-curated release notes or assets. Publish the
draft, delete it, or bump `VERSION` before the next workflow-owned draft is
allowed.

Pull requests do not create releases. Publishing a draft release is a manual
approval step in GitHub.

When bumping the release version, update `VERSION`. CI runs `make version-check`
to ensure the documented release tag and build metadata shape still match the
tracked version.
