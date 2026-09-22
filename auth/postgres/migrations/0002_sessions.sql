CREATE TABLE sessions (
    id         text COLLATE "C" NOT NULL PRIMARY KEY,
    user_id    text COLLATE "C" NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash text COLLATE "C" NOT NULL UNIQUE,
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    last_seen  timestamptz NOT NULL,
    user_agent text NOT NULL DEFAULT '',
    ip         text NOT NULL DEFAULT ''
);

CREATE INDEX sessions_user_id_idx ON sessions (user_id);

CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);
