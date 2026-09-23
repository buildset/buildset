#!/bin/sh
# Creates one schema and one login role per service, the only place the layout is named: migration
# files are unqualified and rely on each role's search_path. It is a shell script rather than .sql
# so the passwords can come from the environment. Postgres runs it once, on an empty data directory.
set -eu

psql --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" --set ON_ERROR_STOP=1 <<SQL
-- Nothing should land in public by accident.
REVOKE ALL ON SCHEMA public FROM PUBLIC;

CREATE ROLE auth_service LOGIN PASSWORD '${AUTH_DB_PASSWORD}';
CREATE ROLE authz_service LOGIN PASSWORD '${AUTHZ_DB_PASSWORD}';
CREATE ROLE content_service LOGIN PASSWORD '${CONTENT_DB_PASSWORD}';

-- Owning its schema is what lets each service run its own migrations at start-up.
CREATE SCHEMA auth AUTHORIZATION auth_service;
CREATE SCHEMA authz AUTHORIZATION authz_service;
CREATE SCHEMA content AUTHORIZATION content_service;

-- search_path deliberately excludes public, so an unqualified reference to another service's
-- table is an error rather than a silent cross-service read.
ALTER ROLE auth_service IN DATABASE ${POSTGRES_DB} SET search_path = auth;
ALTER ROLE authz_service IN DATABASE ${POSTGRES_DB} SET search_path = authz;
ALTER ROLE content_service IN DATABASE ${POSTGRES_DB} SET search_path = content;

-- The single binary connects as the superuser, and tables it creates would otherwise be unreadable
-- by the service role that later connects to the same database.
ALTER DEFAULT PRIVILEGES FOR ROLE ${POSTGRES_USER} IN SCHEMA auth
    GRANT ALL ON TABLES TO auth_service;
ALTER DEFAULT PRIVILEGES FOR ROLE ${POSTGRES_USER} IN SCHEMA authz
    GRANT ALL ON TABLES TO authz_service;
ALTER DEFAULT PRIVILEGES FOR ROLE ${POSTGRES_USER} IN SCHEMA content
    GRANT ALL ON TABLES TO content_service;
SQL
