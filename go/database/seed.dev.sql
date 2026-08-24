\set ON_ERROR_STOP on

\connect cadguanliq

BEGIN;

INSERT INTO users (account, display_name, password_hash, status)
VALUES ('admin', '系统管理员', crypt('admin', gen_salt('bf')), 'active')
ON CONFLICT (account) DO UPDATE
SET display_name = EXCLUDED.display_name,
    status = 'active';

INSERT INTO user_roles (user_id, role)
SELECT id, 'admin'
FROM users
WHERE account = 'admin'
ON CONFLICT (user_id, role) DO NOTHING;

COMMIT;
