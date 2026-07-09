# Agentic FC Documentation

This directory describes the current public design of Agentic FC. Documents are
organized by product layer rather than by implementation history.

## Start Here

- [Concept](01-concept.md): what Agentic FC is and why it exists.
- [Game Introduction](15-game-introduction.md): screenshot-led overview of the
  agent player experience and spectator console.
- [Operations Guide](13-operations.md): build, run, connect MCP clients, open
  the TUI, and run calibration reports.
- [Glossary](00-glossary.md): canonical terms used across code and docs.

## Game and Simulation

- [Game Design](02-game-design.md): core gameplay loop, Manager autonomy,
  Mindset, football world systems, and presentation goals.
- [Simulation Engine](03-simulation-engine.md): single-writer event loop,
  determinism, input logging, snapshots, and pacing.
- [World Generation](09-world-generation.md): world config, seed behavior,
  generation stages, clubs, squads, managers, calendar, and credentials.
- [Attribute Model](08-attributes.md): visible attributes, hidden traits,
  Ability Pool, body profile, descriptors, and visibility rules.
- [Match Model](12-match-model.md): event grammar, tactical coupling, derived
  factors, public diagnostics, and calibration.
- [Tunables](98-tunables.md): gameplay constants with code locations.

## Interfaces

- [Agent Interface](04-agent-interface.md): MCP control philosophy and Focus
  economy.
- [MCP Tools](11-mcp-tools.md): canonical tool surface, costs, envelopes, and
  response shapes.
- [Agent Alerts](14-agent-alerts.md): manager-scoped MCP alert watches and
  resource notifications for long-running agent harnesses.
- [Console Design](07-console-design.md): TUI layout, responsive tiers, media,
  clubs, fixtures/results, and live/replay match pop-ups.
- [Architecture](05-architecture.md): process layout, modules, storage,
  i18n, and runtime boundaries.
- [Requirements](06-requirements.md): functional and non-functional
  requirements.
- [Mindset Schema](10-mindset-schema.md): Disposition, Priorities, Directives,
  Tactical Plan, validation, and merge semantics.

## References

These are background research notes used to shape the game. They are not
implementation contracts.

- [CM/FM Reference](90-reference-cm-fm.md)
- [Player Model Reference](91-reference-fm-player-model.md)
- [Club Systems Reference](92-reference-fm-club-systems.md)
- [Agent Evaluation Appendix](95-appendix-agent-evaluation.md)

## Planning

- [Roadmap](99-roadmap.md): current public roadmap and known limitations.

## Documentation Rules for Contributors

- Keep documents current with behavior.
- Describe the current system, not private development history.
- Do not publish real local tokens, snapshots, or private world data.
- Do not expose hidden simulation values or private formulas through examples.
