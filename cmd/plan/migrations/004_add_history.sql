-- Historial de cambios de estado para todos
CREATE TABLE todos_history (
    id          INTEGER PRIMARY KEY,
    todo_id     INTEGER NOT NULL,
    field       TEXT NOT NULL,
    old_value   TEXT,
    new_value   TEXT,
    changed_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    source      TEXT
);

CREATE INDEX idx_todos_history_todo_id ON todos_history(todo_id);
CREATE INDEX idx_todos_history_changed_at ON todos_history(changed_at);

-- Historial similar para investigations
CREATE TABLE investigations_history (
    id              INTEGER PRIMARY KEY,
    investigation_id INTEGER NOT NULL,
    field           TEXT NOT NULL,
    old_value       TEXT,
    new_value       TEXT,
    changed_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    source          TEXT
);

CREATE INDEX idx_investigations_history_id ON investigations_history(investigation_id);
