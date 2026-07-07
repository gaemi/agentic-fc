# Security Policy

Agentic FC is a local-first simulation server. It can expose an MCP endpoint,
a Console API, persistent world data, Admin tokens, Manager tokens, snapshots,
and audit/input logs.

## Supported Versions

The project is pre-stable. Security fixes are applied to `main` unless a
separate release branch is announced.

## Reporting a Vulnerability

Please do not open a public issue for vulnerabilities.

Report privately through GitHub Security Advisories:

<https://github.com/gaemi/agentic-fc/security/advisories/new>

Repository maintainers should keep GitHub Private Vulnerability Reporting
enabled before making the repository public. Include:

- affected commit or version,
- reproduction steps,
- expected impact,
- whether tokens, snapshots, MCP auth, or local files are involved.

You should receive an acknowledgement when the report is seen. Fixes and
disclosure timing will be coordinated case by case.

## Local Secret Handling

Do not commit or share:

- `admin.token`,
- `manifest.json` from a real world,
- Manager tokens,
- snapshots from worlds that should stay private,
- `audit.jsonl` or input logs containing private test data.

The default `.gitignore` excludes local data directories, but check your diff
before publishing.

## Network Exposure

The daemon defaults to loopback addresses:

- Console API: `127.0.0.1:7420`
- MCP HTTP: `127.0.0.1:7421`

If you bind either endpoint to a public interface, place it behind appropriate
transport security and access controls. Manager tokens authorize gameplay
actions for the bound Manager, and Admin tokens can control world operation.
