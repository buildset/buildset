CREATE TABLE posts (
    id           TEXT NOT NULL PRIMARY KEY,
    -- A reference to a user owned by another service. No foreign key: that service may live in
    -- another database, and this one never resolves the reference.
    author_ref   TEXT NOT NULL,
    title        TEXT NOT NULL,
    body         TEXT NOT NULL DEFAULT '',
    content_type TEXT NOT NULL DEFAULT 'text/plain',
    status       TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL,
    published_at TEXT,
    CHECK (status IN ('draft', 'published', 'archived')),
    -- Extending this list is a one-line migration, and until then an unsupported body format
    -- cannot reach the table.
    CHECK (content_type IN ('text/plain')),
    CHECK (length(title) BETWEEN 1 AND 200),
    CHECK (status <> 'published' OR published_at IS NOT NULL)
) STRICT;

CREATE INDEX posts_status_created_at_idx ON posts (status, created_at DESC);

CREATE INDEX posts_author_ref_idx ON posts (author_ref);
