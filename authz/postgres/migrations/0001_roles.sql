CREATE TABLE roles (
    name        text COLLATE "C" NOT NULL PRIMARY KEY,
    description text NOT NULL DEFAULT ''
);

CREATE TABLE role_permissions (
    role             text COLLATE "C" NOT NULL REFERENCES roles (name) ON DELETE CASCADE,
    action           text COLLATE "C" NOT NULL,
    resource_pattern text COLLATE "C" NOT NULL,
    PRIMARY KEY (role, action, resource_pattern)
);

-- subject_ref has no foreign key on purpose. This service never resolves a subject and must keep
-- working when the service that owns it lives in another schema or another database.
CREATE TABLE subject_roles (
    subject_ref text COLLATE "C" NOT NULL,
    role        text COLLATE "C" NOT NULL REFERENCES roles (name) ON DELETE CASCADE,
    granted_at  timestamptz NOT NULL,
    PRIMARY KEY (subject_ref, role)
);

CREATE TABLE grants (
    subject_ref  text COLLATE "C" NOT NULL,
    action       text COLLATE "C" NOT NULL,
    resource_ref text COLLATE "C" NOT NULL,
    granted_at   timestamptz NOT NULL,
    PRIMARY KEY (subject_ref, action, resource_ref)
);

CREATE INDEX grants_resource_ref_idx ON grants (resource_ref);

CREATE INDEX subject_roles_role_idx ON subject_roles (role);
