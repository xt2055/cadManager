\set ON_ERROR_STOP on

-- 仅用于本机开发环境。生产环境请使用独立的密钥管理方式。
\connect postgres

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'cadguanliq_app') THEN
        CREATE ROLE cadguanliq_app LOGIN PASSWORD '5201314520xt';
    ELSE
        ALTER ROLE cadguanliq_app LOGIN PASSWORD '5201314520xt';
    END IF;
END
$$;

SELECT format('CREATE DATABASE %I OWNER %I', 'cadguanliq', 'cadguanliq_app')
WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = 'cadguanliq')\gexec

\connect cadguanliq
\ir migrations/000001_initial_schema.sql
\ir migrations/000002_data_document_compat.sql
\ir migrations/000003_review_node_roles.sql
\ir migrations/000004_review_flow_assignments.sql

-- 初始化脚本由管理员执行时，明确授予业务账号运行时权限。
GRANT CONNECT ON DATABASE cadguanliq TO cadguanliq_app;
GRANT USAGE ON SCHEMA public TO cadguanliq_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO cadguanliq_app;
GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA public TO cadguanliq_app;
GRANT EXECUTE ON FUNCTION set_updated_at() TO cadguanliq_app;

ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO cadguanliq_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO cadguanliq_app;
