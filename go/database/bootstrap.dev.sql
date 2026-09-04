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
\ir migrations/000003_review_node_roles.sql
\ir migrations/000004_review_flow_assignments.sql
\ir migrations/000005_drawing_operation_audit.sql
\ir migrations/000006_edit_sessions.sql
\ir migrations/000007_file_versions.sql
\ir migrations/000008_edit_session_tickets.sql
\ir migrations/000009_current_file_storage.sql
\ir migrations/000010_update_management.sql
\ir migrations/000011_drawing_lifecycle.sql
\ir migrations/000012_versioned_edit_work_files.sql
\ir migrations/000013_remove_hidden_status.sql
\ir migrations/000014_modular_domain_data.sql
\ir migrations/000015_remove_data_documents.sql
\ir migrations/000016_attachment_storage_key_lifecycle.sql
\ir migrations/000017_upload_sessions_and_blobs.sql
\ir migrations/000018_upload_chunks.sql
\ir migrations/000019_repair_upload_schema.sql
\ir migrations/000020_resource_revisions.sql
\ir migrations/000021_repair_current_cad_metadata.sql
\ir migrations/000022_rebuild_final_domain_model.sql
\ir migrations/000023_domain_command_idempotency.sql

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
