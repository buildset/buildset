CREATE TABLE users (
    id            TEXT NOT NULL PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    name          TEXT NOT NULL DEFAULT '',
    password_hash TEXT NOT NULL,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    -- Usernames are lowercased in Go; this is the backstop against two accounts differing only
    -- in case.
    CHECK (username = lower(username)),
    CHECK (length(username) BETWEEN 3 AND 32),
    CHECK (length(password_hash) > 0)
) STRICT;
