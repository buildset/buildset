-- Twin of auth/sqlite/migrations/0002_sessions.sql.
CREATE TABLE sessions (
    id         text COLLATE "C" NOT NULL PRIMARY KEY,
    user_id    text COLLATE "C" NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- SHA-256 of the token held by the browser. The token itself is never stored.
    token_hash text COLLATE "C" NOT NULL UNIQUE,
    created_at text COLLATE "C" NOT NULL,
    expires_at text COLLATE "C" NOT NULL,
    last_seen  text COLLATE "C" NOT NULL,
    user_agent text NOT NULL DEFAULT '',
    ip         text NOT NULL DEFAULT ''
);

CREATE INDEX sessions_user_id_idx ON sessions (user_id);

CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);
