CREATE TABLE users (
    -- COLLATE "C" gives byte ordering, matching SQLite, so both backends sort and compare
    -- identically whatever locale the database was created with.
    id            text COLLATE "C" NOT NULL PRIMARY KEY,
    username      text COLLATE "C" NOT NULL UNIQUE,
    name          text NOT NULL DEFAULT '',
    password_hash text NOT NULL,
    created_at    timestamptz NOT NULL,
    updated_at    timestamptz NOT NULL,
    -- Usernames are lowercased in Go before every insert and lookup. These are the backstop that
    -- keeps a future code path from creating two accounts that differ only in case.
    CONSTRAINT users_username_is_lower CHECK (username = lower(username)),
    CONSTRAINT users_username_length CHECK (char_length(username) BETWEEN 3 AND 32),
    CONSTRAINT users_password_hash_present CHECK (char_length(password_hash) > 0)
);
