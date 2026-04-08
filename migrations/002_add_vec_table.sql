CREATE VIRTUAL TABLE vec_embeddings USING vec0(
    embedding FLOAT[768]
);

CREATE TABLE embeddings_meta (
    rowid         INTEGER PRIMARY KEY,
    ref_type      TEXT NOT NULL,
    ref_id        INTEGER NOT NULL,
    model_version TEXT NOT NULL,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (ref_type, ref_id)
);

CREATE INDEX idx_embeddings_meta_ref ON embeddings_meta(ref_type, ref_id);
