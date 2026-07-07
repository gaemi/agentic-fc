# Roadmap

This document lists current public priorities and known limitations. It is not
a development-history log.

## Current Focus

Agentic FC is playable through MCP and watchable through the TUI. The next
improvements are about depth, polish, and operational hardening rather than
establishing the core loop.

## Simulation Depth

- More position-aware squad selection, bench construction, and replacement
  logic.
- Suspensions and disciplinary consequences after red cards.
- Loans, pre-contracts, signing bonuses, and richer wage/contract negotiation.
- More detailed transfer needs by position group and squad role.
- Continental competitions and broader football calendar structures.
- Archive retention controls for long-running worlds.

## Match Model

- More event families beyond the current key chance types.
- Published event-family weight tables once the model stabilizes.
- More public post-match explanatory summaries built from observed diagnostics.
- Long-run balance checks across presets, quality bands, and run profiles.
- More commentary variation for different tactical and emotional contexts.

## Agent Experience

- Stronger examples for common MCP clients.
- More structured playbooks for common goals such as title challenge, survival,
  youth development, and financial rebuild.
- Better explanation of Focus tradeoffs for agents watching other clubs.

## Console Experience

- Lineups and substitution log section on the live match screen.
- More flexible layouts for very short and extra-tall terminals.
- Additional club, manager, and player history screens.
- More browsing tools for archived seasons.

## Operations

- Release packaging for major platforms.
- Clearer example service files for long-running local worlds.
- Snapshot/export tooling for sharing reproducible demo worlds without tokens.
- Optional retention and compaction tools for audit/input logs.

## Stability Before a Public Release

- Keep save and API changes explicit.
- Keep docs aligned with current behavior.
- Keep deterministic invariants covered by tests.
- Avoid exposing hidden attributes or exact private formulas through public
  interfaces.
