# Contributing to Agentic FC

Thank you for considering a contribution. Agentic FC is a simulation-heavy
project, so correctness is not only "does it compile"; changes must preserve
determinism, visibility boundaries, and the game contract described in `docs/`.

## Development Setup

Requirements:

- Go 1.26 or newer
- Git
- A UTF-8 terminal for running the TUI

Useful commands:

```sh
make fmt
make verify
make security
make vet
make test
make build
```

Before opening a pull request, run:

```sh
make verify
make security
```

CI also runs Markdown link checks, GitHub Actions linting, `govulncheck`, and a
secret scan. Do not commit local worlds, tokens, generated binaries, or private
logs.

## Documentation Contract

The design documents in `docs/` are part of the project contract. If behavior,
wire shape, configuration, or gameplay rules change, update the relevant docs
in the same pull request.

Frequently touched docs:

- `docs/10-mindset-schema.md` for Mindset, Directives, and Tactical Plan.
- `docs/11-mcp-tools.md` for MCP tool contracts and Focus costs.
- `docs/12-match-model.md` for match-model changes.
- `docs/98-tunables.md` for gameplay constants and balance values.
- `docs/99-roadmap.md` for public roadmap updates.

## Simulation Rules

Contributions must preserve these invariants:

- The simulation core is single-writer.
- All randomness that affects world state flows through internal RNG streams.
- Same seed, config, snapshot, queue, and input log should reproduce the same
  trajectory.
- Accepted MCP calls are logged in total order.
- No floats or non-deterministic values should enter persisted world-hash state.

When changing deterministic behavior, add or update tests that lock the new
invariant.

## Visibility Rules

MCP is the play surface. It may expose public facts, scouting uncertainty,
descriptors, evidence, and controllable intent state. It must not expose:

- hidden raw attributes,
- exact private formulas,
- seed/replay randomness inputs,
- internal engine weights that are not part of the public contract.

The Console/TUI is a spectator surface and can show richer public observations,
but it follows the same no-hidden-value rule.

## Text and Localization

All human-facing text uses message keys through `internal/narrative`.

When adding a new key:

- add entries for every currently supported locale catalog in the same change,
- keep parameters deterministic and serializable,
- avoid putting floats or private raw values into commentary/news params.

## Dependencies

Keep dependencies small and intentional. If a new dependency is required:

- explain why the standard library or existing project dependencies are not
  sufficient,
- pin it through `go.mod`,
- run `go mod tidy`,
- update docs if it changes build or runtime expectations.

## Pull Request Checklist

- [ ] Code builds: `go build ./...`
- [ ] Tests pass: `go test ./...`
- [ ] Vet passes: `go vet ./...`
- [ ] Formatting is clean: `gofmt -l .` returns nothing
- [ ] Markdown links resolve
- [ ] GitHub Actions changes pass `actionlint`
- [ ] Security-sensitive changes considered `govulncheck` and secret-scan output
- [ ] Docs updated for contract changes
- [ ] New human-facing text has entries for every currently supported locale catalog
- [ ] Tunables are registered in `docs/98-tunables.md`
- [ ] Visibility boundaries are preserved

## Issue Reports

Helpful bug reports include:

- expected behavior,
- actual behavior,
- reproduction steps,
- command-line flags,
- seed/preset/profile if relevant,
- whether the issue affects daemon, MCP, Console API, or TUI.

Do not include real Admin or Manager tokens in public issues.
