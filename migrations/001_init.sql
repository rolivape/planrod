-- Todos: cosas pendientes con estado mutable
CREATE TABLE todos (
    id              INTEGER PRIMARY KEY,
    title           TEXT NOT NULL,
    content         TEXT,
    status          TEXT NOT NULL DEFAULT 'pending',
    priority        TEXT,
    deadline        DATETIME,
    related_entities TEXT,
    related_specs    TEXT,
    investigation_id INTEGER REFERENCES investigations(id) ON DELETE SET NULL,
    session_id       INTEGER REFERENCES sessions(id) ON DELETE SET NULL,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    completed_at    DATETIME
);

CREATE INDEX idx_todos_status ON todos(status);
CREATE INDEX idx_todos_priority ON todos(priority);
CREATE INDEX idx_todos_deadline ON todos(deadline);
CREATE INDEX idx_todos_investigation_id ON todos(investigation_id);
CREATE INDEX idx_todos_session_id ON todos(session_id);

-- Decisions: registro de decisiones tomadas
CREATE TABLE decisions (
    id              INTEGER PRIMARY KEY,
    title           TEXT NOT NULL,
    choice          TEXT NOT NULL,
    why             TEXT,
    alternatives    TEXT,
    category        TEXT,
    related_entities TEXT,
    related_specs    TEXT,
    invalidates_decision_id INTEGER REFERENCES decisions(id) ON DELETE SET NULL,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_decisions_category ON decisions(category);
CREATE INDEX idx_decisions_invalidates ON decisions(invalidates_decision_id);
CREATE INDEX idx_decisions_created_at ON decisions(created_at);

-- Investigations: hipótesis abiertas con evidencia acumulándose
CREATE TABLE investigations (
    id              INTEGER PRIMARY KEY,
    name            TEXT NOT NULL UNIQUE,
    hypothesis      TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'open',
    conclusion      TEXT,
    evidence        TEXT,
    related_entities TEXT,
    related_specs    TEXT,
    opened_at       DATETIME DEFAULT CURRENT_TIMESTAMP,
    closed_at       DATETIME
);

CREATE INDEX idx_investigations_status ON investigations(status);
CREATE INDEX idx_investigations_name ON investigations(name);

-- Sessions: snapshots de conversaciones de trabajo
CREATE TABLE sessions (
    id              INTEGER PRIMARY KEY,
    title           TEXT,
    summary         TEXT NOT NULL,
    summary_path    TEXT,
    decisions_made  TEXT,
    todos_opened    TEXT,
    todos_closed    TEXT,
    related_entities TEXT,
    related_specs    TEXT,
    source          TEXT,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_sessions_source ON sessions(source);
CREATE INDEX idx_sessions_created_at ON sessions(created_at);
