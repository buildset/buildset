CREATE TABLE users (
    -- COLLATE "C" gives byte ordering, so both backends sort identically whatever locale the
    -- database was created with.
    id            text COLLATE "C" NOT NULL PRIMARY KEY,
    username      text COLLATE "C" NOT NULL UNIQUE,
    name          text NOT NULL DEFAULT '',
    password_hash text NOT NULL,
    created_at    timestamptz NOT NULL,
    updated_at    timestamptz NOT NULL,
    -- Usernames are lowercased in Go; this is the backstop against two accounts differing only
    -- in case.
    CONSTRAINT users_username_is_lower CHECK (username = lower(username)),
    CONSTRAINT users_username_length CHECK (char_length(username) BETWEEN 3 AND 32),
    CONSTRAINT users_password_hash_present CHECK (char_length(password_hash) > 0)
);
