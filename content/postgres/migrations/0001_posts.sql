CREATE TABLE posts (
    id           text COLLATE "C" NOT NULL PRIMARY KEY,
    -- A reference to a user owned by another service. No foreign key: that service may live in
    -- another schema or another database, and this one never resolves the reference.
    author_ref   text COLLATE "C" NOT NULL,
    title        text NOT NULL,
    body         text NOT NULL DEFAULT '',
    content_type text COLLATE "C" NOT NULL DEFAULT 'text/plain',
    status       text COLLATE "C" NOT NULL,
    created_at   timestamptz NOT NULL,
    updated_at   timestamptz NOT NULL,
    published_at timestamptz,
    CONSTRAINT posts_status_known CHECK (status IN ('draft', 'published', 'archived')),
    CONSTRAINT posts_content_type_known CHECK (content_type IN ('text/plain')),
    CONSTRAINT posts_title_length CHECK (char_length(title) BETWEEN 1 AND 200),
    CONSTRAINT posts_published_has_timestamp CHECK (status <> 'published' OR published_at IS NOT NULL)
);

CREATE INDEX posts_status_created_at_idx ON posts (status, created_at DESC);

CREATE INDEX posts_author_ref_idx ON posts (author_ref);
