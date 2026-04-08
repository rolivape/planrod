CREATE VIRTUAL TABLE fts_todos USING fts5(
    title, content,
    content='todos',
    content_rowid='id'
);

CREATE VIRTUAL TABLE fts_decisions USING fts5(
    title, choice, why, category,
    content='decisions',
    content_rowid='id'
);

CREATE VIRTUAL TABLE fts_investigations USING fts5(
    name, hypothesis, conclusion,
    content='investigations',
    content_rowid='id'
);

CREATE VIRTUAL TABLE fts_sessions USING fts5(
    title, summary,
    content='sessions',
    content_rowid='id'
);

-- Triggers for todos
CREATE TRIGGER todos_ai AFTER INSERT ON todos BEGIN
    INSERT INTO fts_todos(rowid, title, content) VALUES (new.id, new.title, new.content);
END;

CREATE TRIGGER todos_ad AFTER DELETE ON todos BEGIN
    INSERT INTO fts_todos(fts_todos, rowid, title, content) VALUES('delete', old.id, old.title, old.content);
END;

CREATE TRIGGER todos_au AFTER UPDATE ON todos BEGIN
    INSERT INTO fts_todos(fts_todos, rowid, title, content) VALUES('delete', old.id, old.title, old.content);
    INSERT INTO fts_todos(rowid, title, content) VALUES (new.id, new.title, new.content);
END;

-- Triggers for decisions
CREATE TRIGGER decisions_ai AFTER INSERT ON decisions BEGIN
    INSERT INTO fts_decisions(rowid, title, choice, why, category) VALUES (new.id, new.title, new.choice, new.why, new.category);
END;

CREATE TRIGGER decisions_ad AFTER DELETE ON decisions BEGIN
    INSERT INTO fts_decisions(fts_decisions, rowid, title, choice, why, category) VALUES('delete', old.id, old.title, old.choice, old.why, old.category);
END;

CREATE TRIGGER decisions_au AFTER UPDATE ON decisions BEGIN
    INSERT INTO fts_decisions(fts_decisions, rowid, title, choice, why, category) VALUES('delete', old.id, old.title, old.choice, old.why, old.category);
    INSERT INTO fts_decisions(rowid, title, choice, why, category) VALUES (new.id, new.title, new.choice, new.why, new.category);
END;

-- Triggers for investigations
CREATE TRIGGER investigations_ai AFTER INSERT ON investigations BEGIN
    INSERT INTO fts_investigations(rowid, name, hypothesis, conclusion) VALUES (new.id, new.name, new.hypothesis, new.conclusion);
END;

CREATE TRIGGER investigations_ad AFTER DELETE ON investigations BEGIN
    INSERT INTO fts_investigations(fts_investigations, rowid, name, hypothesis, conclusion) VALUES('delete', old.id, old.name, old.hypothesis, old.conclusion);
END;

CREATE TRIGGER investigations_au AFTER UPDATE ON investigations BEGIN
    INSERT INTO fts_investigations(fts_investigations, rowid, name, hypothesis, conclusion) VALUES('delete', old.id, old.name, old.hypothesis, old.conclusion);
    INSERT INTO fts_investigations(rowid, name, hypothesis, conclusion) VALUES (new.id, new.name, new.hypothesis, new.conclusion);
END;

-- Triggers for sessions
CREATE TRIGGER sessions_ai AFTER INSERT ON sessions BEGIN
    INSERT INTO fts_sessions(rowid, title, summary) VALUES (new.id, new.title, new.summary);
END;

CREATE TRIGGER sessions_ad AFTER DELETE ON sessions BEGIN
    INSERT INTO fts_sessions(fts_sessions, rowid, title, summary) VALUES('delete', old.id, old.title, old.summary);
END;

CREATE TRIGGER sessions_au AFTER UPDATE ON sessions BEGIN
    INSERT INTO fts_sessions(fts_sessions, rowid, title, summary) VALUES('delete', old.id, old.title, old.summary);
    INSERT INTO fts_sessions(rowid, title, summary) VALUES (new.id, new.title, new.summary);
END;
