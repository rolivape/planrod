# PlanRod

HiveRod Cognitive Planning Service (HCC). Manages todos, decisions, investigations, and sessions.

## Build

```bash
go build -tags "sqlite_fts5 sqlite_load_extension" -o /opt/plan/bin/plan ./cmd/plan
```

## Run

```bash
# Start MCP server
plan serve --mcp-port 9101 --metrics-port 9111

# Run migrations
plan migrate --db /opt/plan/data/plan.db

# CLI usage
plan add todo "Fix something" --priority high
plan complete todo 1
plan list todos --status pending
plan record decision "Use SQLite" --choice "SQLite with WAL" --why "Consistency with HCC stack"
plan search "tunnel memoryrod"
```

## Cross-service mode

PlanRod supports cross-service validation against MemoryRod:

- `--cross-service-mode none` — ignore entity/spec refs
- `--cross-service-mode lite` (default) — store refs as strings, no validation
- `--cross-service-mode strict` — validate against MemoryRod before storing

Per-call override via `_options.cross_service_mode` field.

## Import from Cortex v2

```bash
plan import-cortex --cortex-db /tmp/cortex-migration-copy.db --dry-run
plan import-cortex --cortex-db /tmp/cortex-migration-copy.db --apply
```

## License

MIT
