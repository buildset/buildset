-- These three roles are the blog's vocabulary, seeded here because the service needs a stable set
-- to assign from. Nothing in authz's code knows these names.
--
-- TODO: move the seed out to the application once roles are managed through an interface, so this
-- generic service stops shipping one application's role names.
INSERT INTO roles (name, description) VALUES
    ('admin', 'Full access to every action and resource'),
    ('author', 'May create posts, and holds direct grants over the posts they create'),
    ('reader', 'Signed-in reader with no elevated permissions');

-- reader deliberately gets no rows: published posts are public, so nothing asks authz about them.
INSERT INTO role_permissions (role, action, resource_pattern) VALUES
    ('admin', '*', '*'),
    ('author', 'post.create', 'urn:content:post:*');
