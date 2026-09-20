CREATE TABLE users (
    id            TEXT NOT NULL PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    name          TEXT NOT NULL DEFAULT '',
    password_hash TEXT NOT NULL,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    -- Usernames are lowercased in Go before every insert and lookup. These are the backstop that
    -- keeps a future code path from creating two accounts that differ only in case.
    CHECK (username = lower(username)),
    CHECK (length(username) BETWEEN 3 AND 32),
    CHECK (length(password_hash) > 0)
) STRICT;
