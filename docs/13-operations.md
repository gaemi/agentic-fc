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
  -start
```

Useful daemon flags:

| Flag | Default | Meaning |
|------|---------|---------|
| `-data` | `./data` | Data directory for snapshot, manifest, logs, and tokens. |
| `-console-addr` | `127.0.0.1:7420` | Console API listen address. |
| `-mcp-addr` | `127.0.0.1:7421` | MCP Streamable HTTP listen address. |
| `-preset` | `classic` | New-world preset: `compact`, `classic`, `deep`, `sprawling`. |
| `-seed` | `0` | New-world seed; `0` means random. |
| `-world-name` | generated | Display name for a new world. |
| `-profile` | `default` | Run profile: `default`, `fast`, `slow`, `custom`. |
| `-speed` | profile value | Match-window speed override: `5`, `15`, `30`, or `60`. |
| `-idle-accel` | profile value | In-season idle acceleration multiplier. |
| `-offseason-accel` | profile value | Off-season acceleration multiplier. |
| `-start` | `false` | Start simulation immediately. |
| `-snapshot-interval` | `1m` | Periodic snapshot cadence. |
| `-widget-mode` | `apps` | MCP UI mode: `apps`, `meta`, or `content`. |

Generation flags apply only when no world exists in the data directory. A
subsequent daemon run resumes the existing world.

## Data Directory

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
| `-admin-token` | Reserved for Admin Mode screens. |

The console is read-only today: it watches the world through public spectator
views.

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

## Automated Prereleases

The `prerelease` GitHub Actions workflow publishes packaged prereleases from
`main`. It is intentionally downstream of CI:

1. A commit lands on `main`.
2. The `ci` workflow runs for that push.
3. If CI succeeds, the `prerelease` workflow checks out the exact passing commit
   SHA.
4. The workflow reads the base version from the root `VERSION` file and computes
   a SemVer prerelease tag: `v0.1.0-pre.<commit_count>.g<short_sha>`.
5. It cross-compiles all shipped commands and creates a GitHub Release marked as
   a prerelease.

The prerelease packages include:

- `agenticfc`
- `agenticfc-console`
- `agenticfc-calibrate`
- `README.md`, `CHANGELOG.md`, and `LICENSE`

Each binary supports `--version`. Release builds inject the prerelease tag and
commit SHA into that output so bug reports can be traced back to the published
artifact.

Published target triples:

| OS | Architectures | Archive |
|----|---------------|---------|
| Linux | `amd64`, `arm64` | `.tar.gz` |
| macOS | `amd64`, `arm64` | `.tar.gz` |
| Windows | `amd64`, `arm64` | `.zip` |

The release also includes `checksums.txt` with SHA-256 hashes for every archive
and release notes linking back to the CI run and prerelease build run.

Prereleases are not stable API or save-format commitments. They are intended for
early testing of the current `main` branch. A failed CI run does not publish a
release, and pull requests do not publish releases. Re-running CI for the same
commit is idempotent: the prerelease tag is derived from the commit, so the
workflow re-uploads assets with `--clobber` when that release already exists
instead of creating a duplicate prerelease.

When bumping the prerelease base, update `VERSION`. CI runs `make
version-check` to ensure the documented prerelease tag shape still matches the
tracked base version.
