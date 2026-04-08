# Changelog

## [0.1.0] - 2026-04-08

### Added
- Initial release of PlanRod
- 16 MCP tools: todos (5), decisions (3), investigations (4), sessions (2), cross-cutting (2)
- SQLite + FTS5 + sqlite-vec for hybrid search
- Cross-service validation against MemoryRod (lite/strict/none modes)
- CLI with Cobra (add, complete, list, get, search, record, open, close, log)
- Sessions filesystem-first with git versioning (>2500 char threshold)
- History tables for todos and investigations
- import-cortex subcommand for Cortex v2 migration
- Systemd service unit
- Caddy reverse proxy config for plan.hiverod.com
