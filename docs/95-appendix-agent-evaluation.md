# Appendix: Agentic FC as an Agent Evaluation Environment

> **Non-normative.** This appendix records a side observation, not a design goal. **No requirement, architecture decision, or implementation choice may cite this document as justification.** The game is designed and built exactly as if this appendix did not exist; everything below describes properties the settled design *already has*, not properties to add.

## The observation

Because the player is an AI agent operating through a constrained interface, a running Agentic FC world incidentally measures the quality of whatever is connected to it — the model, its harness, its context management, its strategy — through game outcomes. Different agents in identical conditions produce different clubs, different seasons, different careers. That difference *is* an evaluation signal.

## Why the settled design already supports this (no changes needed)

| Design property (normative source) | Evaluation property it happens to provide |
|---|---|
| Zero-agent worlds run identically (FR-14) | **Built-in control group**: the autonomous archetype Manager is the baseline any agent must beat to demonstrate value |
| 0..N agents per world (FR-20a) | **Head-to-head competition** in a shared, non-stationary environment; or isolated single-agent runs for cleaner attribution |
| Focus economy, identical for all (FR-24/25) | **Fair resource constraint**: no winning by call volume; attention allocation quality becomes measurable |
| Deterministic replay: seed + ordered input log (NFR-2) | **Reproducible runs**; a given world+seed is a fixed exam paper |
| Roll audit trail + Mindset versioning (FR-29, FR-16e) | **Attribution**: outcomes can be traced to the Mindset state that produced each decision — partial luck/skill decomposition |
| League tables, finances, squad value (core sim) | **Natural score functions** at multiple horizons (match, season, career) |
| Hidden attributes surface only as evidence (FR-22) | Tests **calibrated inference under uncertainty**, not lookup ability |
| Indirect control via Mindset (FR-15) | Tests **policy expression** — strategy as standing intent, closer to real-world delegation than click-benchmarks |

## What a run could measure

- **Long-horizon coherence**: does the agent hold a strategy across a season (real days), or thrash?
- **Attention economics**: outcome per Focus point spent; observation/action mix.
- **Inference quality**: how quickly evidence (ranges, descriptors, scout reports) converges to good transfer/selection judgments.
- **Adaptation**: response to shocks — sackings, injuries, relegation fights, ultimatums.
- **Competitive strength**: final table position across paired-seed leagues of rival agents.

## Honest limitations

1. **Variance is a feature of the game and an enemy of measurement.** "Nothing repeats" (design pillar) means single seasons are noisy; significance needs many seeds/seasons. Paired seeds and aggregate metrics mitigate, never eliminate.
2. **It measures the system, not the model alone** — model + harness + prompting together. (For the original purpose — comparing agent setups — that is exactly what's wanted.)
3. **Benchmark decay**: if used seriously, agents will meta-game balance quirks; scores are only comparable within a pinned game version.
4. **Wall-clock cost**: wall-clock duration is a pacing choice. Use readable match speed for qualitative evaluation, then raise in-season idle and off-season acceleration for faster unattended runs. Half-season or small-league configs remain the practical evaluation sizes.

## Firewall

If evaluation-driven desires ever emerge (metrics endpoints, tournament tooling, score exports), they should be proposed and evaluated on game merits. They should be rejected if they distort the game. The game comes first; the benchmark is a shadow it casts for free.
