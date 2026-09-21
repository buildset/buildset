-- Twin of auth/sqlite/migrations/0001_users.sql. The two must stay in step: a change to a column
-- or a constraint here needs the same change there.
--
-- COLLATE "C" pins byte ordering, matching SQLite, so the two backends sort and compare
-- identically regardless of the database locale.
CREATE TABLE users (
    id            text COLLATE "C" NOT NULL PRIMARY KEY,
    username      text COLLATE "C" NOT NULL UNIQUE,
    name          text NOT NULL DEFAULT '',
    password_hash text NOT NULL,
    created_at    text COLLATE "C" NOT NULL,
    updated_at    text COLLATE "C" NOT NULL,
    -- Usernames are lowercased in Go before every insert and lookup. These are the backstop that
    -- keeps a future code path from creating two accounts that differ only in case.
    CONSTRAINT users_username_is_lower CHECK (username = lower(username)),
    CONSTRAINT users_username_length CHECK (char_length(username) BETWEEN 3 AND 32),
    CONSTRAINT users_password_hash_present CHECK (char_length(password_hash) > 0)
);
