# PlanRod Schema

## Tables

### todos
| Column | Type | Notes |
|--------|------|-------|
| id | INTEGER PK | |
| title | TEXT NOT NULL | |
| content | TEXT | |
| status | TEXT | pending, in_progress, done, cancelled |
| priority | TEXT | low, medium, high, urgent |
| deadline | DATETIME | |
| related_entities | TEXT | JSON array |
| related_specs | TEXT | JSON array |
| investigation_id | INTEGER FK | → investigations(id) |
| session_id | INTEGER FK | → sessions(id) |
| created_at | DATETIME | |
| updated_at | DATETIME | |
| completed_at | DATETIME | |

### decisions
| Column | Type | Notes |
|--------|------|-------|
| id | INTEGER PK | |
| title | TEXT NOT NULL | |
| choice | TEXT NOT NULL | |
| why | TEXT | |
| alternatives | TEXT | JSON array |
| category | TEXT | |
| related_entities | TEXT | JSON array |
| related_specs | TEXT | JSON array |
| invalidates_decision_id | INTEGER FK | → decisions(id) |
| created_at | DATETIME | |

### investigations
| Column | Type | Notes |
|--------|------|-------|
| id | INTEGER PK | |
| name | TEXT UNIQUE NOT NULL | |
| hypothesis | TEXT NOT NULL | |
| status | TEXT | open, in_progress, closed, abandoned |
| conclusion | TEXT | |
| evidence | TEXT | JSON array |
| related_entities | TEXT | JSON array |
| related_specs | TEXT | JSON array |
| opened_at | DATETIME | |
| closed_at | DATETIME | |

### sessions
| Column | Type | Notes |
|--------|------|-------|
| id | INTEGER PK | |
| title | TEXT | |
| summary | TEXT NOT NULL | |
| summary_path | TEXT | path in sessions/ if >2500 chars |
| decisions_made | TEXT | JSON array of decision IDs |
| todos_opened | TEXT | JSON array of todo IDs |
| todos_closed | TEXT | JSON array of todo IDs |
| related_entities | TEXT | JSON array |
| related_specs | TEXT | JSON array |
| source | TEXT | claude-ai, claude-code, grok, manual |
| created_at | DATETIME | |

### todos_history / investigations_history
Audit trail for state changes. See migrations/004_add_history.sql.

### vec_embeddings + embeddings_meta
Polimorphic vector embeddings. See migrations/002_add_vec_table.sql.

### FTS5 virtual tables
fts_todos, fts_decisions, fts_investigations, fts_sessions with triggers.
See migrations/003_add_fts.sql.
