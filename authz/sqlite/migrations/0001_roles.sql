CREATE TABLE roles (
    name        TEXT NOT NULL PRIMARY KEY,
    description TEXT NOT NULL DEFAULT ''
) STRICT;

CREATE TABLE role_permissions (
    role             TEXT NOT NULL REFERENCES roles (name) ON DELETE CASCADE,
    action           TEXT NOT NULL,
    resource_pattern TEXT NOT NULL,
    PRIMARY KEY (role, action, resource_pattern)
) STRICT, WITHOUT ROWID;

-- subject_ref has no foreign key on purpose. This service never resolves a subject and must keep
-- working when the service that owns it lives in another database.
CREATE TABLE subject_roles (
    subject_ref TEXT NOT NULL,
    role        TEXT NOT NULL REFERENCES roles (name) ON DELETE CASCADE,
    granted_at  TEXT NOT NULL,
    PRIMARY KEY (subject_ref, role)
) STRICT, WITHOUT ROWID;

CREATE TABLE grants (
    subject_ref  TEXT NOT NULL,
    action       TEXT NOT NULL,
    resource_ref TEXT NOT NULL,
    granted_at   TEXT NOT NULL,
    PRIMARY KEY (subject_ref, action, resource_ref)
) STRICT, WITHOUT ROWID;

CREATE INDEX grants_resource_ref_idx ON grants (resource_ref);

CREATE INDEX subject_roles_role_idx ON subject_roles (role);
