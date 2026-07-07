# Agentic FC Agent Guide

This file gives coding agents and contributors the project rules needed to
work safely in this repository.

Agentic FC is a football management simulation played by AI agents through MCP
and watched by humans through a TUI Console. The Go module is
`github.com/gaemi/agentic-fc`.

## Commands

- Build: `make build` or `go build ./...`
- Test: `make test` or `go test ./...`
- Format: `make fmt` or `gofmt -w .`
- Vet: `make vet` or `go vet ./...`

Before finishing a code change, run:

```sh
go test ./...
go vet ./...
go build ./...
test -z "$(gofmt -l .)"
```

## Documentation Contract

The public docs in `docs/` describe the current system and are part of the
project contract.

Important docs:

- `docs/00-glossary.md`: canonical terms.
- `docs/03-simulation-engine.md`: single-writer engine, determinism, replay.
- `docs/08-attributes.md`: visible/hidden attributes and descriptors.
- `docs/09-world-generation.md`: world config and generation.
- `docs/10-mindset-schema.md`: Mindset, Directives, Tactical Plan.
- `docs/11-mcp-tools.md`: MCP tool surface and Focus costs.
- `docs/12-match-model.md`: match model and diagnostics.
- `docs/98-tunables.md`: gameplay constants and code locations.
- `docs/99-roadmap.md`: public roadmap and known limitations.

When code and docs disagree, update both in the same change.

## Core Invariants

- The simulation core is single-writer.
- Randomness that affects world state flows through internal RNG streams.
- Accepted MCP calls go through the input log in total order.
- Hidden attributes and exact private formulas do not cross MCP, Console API,
  TUI, logs, or public examples.
- Human-facing strings use `internal/narrative` message keys.
- New human-facing keys need both English and Korean catalog entries.
- Gameplay tunables must be registered in `docs/98-tunables.md`.

## Public Interface Boundaries

- MCP is the play surface. It exposes public facts, scouting uncertainty,
  Focus state, news, and Mindset/Tactical Plan controls.
- Console/TUI is the spectator surface. It may show richer public observations,
  but never hidden raw values.
- Local tokens, manifests, snapshots, and logs from real worlds are not
  suitable for public examples.

## Project Layout

```text
cmd/agenticfc/             daemon: simulation, MCP, Console API
cmd/agenticfc-console/     Bubble Tea spectator console
cmd/agenticfc-calibrate/   deterministic match calibration CLI
internal/engine/           single-writer simulation engine
internal/worldgen/         world configuration and generated state
internal/mcpserver/        MCP gateway
internal/consoleapi/       HTTP/SSE console API
internal/tui/              terminal UI
internal/narrative/        localized text catalogs
docs/                      design and operations docs
```

## Editing Guidance

- Prefer existing patterns over new abstractions.
- Keep changes scoped to the requested behavior.
- Add tests for deterministic behavior, visibility boundaries, and API shape.
- Keep comments useful and sparse.
- Do not commit generated local world data.
