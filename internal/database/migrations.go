package database

type Migration struct {
	Version     int
	Description string
	SQL         string
}

const BaselineCatalogDDL = `
CREATE TABLE IF NOT EXISTS sys_tenants (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    name VARCHAR(128) NOT NULL,
    slug VARCHAR(64) NOT NULL UNIQUE,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS sys_roles (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id BIGINT NOT NULL REFERENCES sys_tenants(id) ON DELETE CASCADE,
    name VARCHAR(64) NOT NULL,
    description TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_roles_tenant_name UNIQUE (tenant_id, name)
);

CREATE TABLE IF NOT EXISTS sys_users (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    tenant_id BIGINT NOT NULL REFERENCES sys_tenants(id) ON DELETE CASCADE,
    email VARCHAR(255) NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    role_id BIGINT NULL REFERENCES sys_roles(id) ON DELETE SET NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_users_tenant_email UNIQUE (tenant_id, email)
);

CREATE TABLE IF NOT EXISTS sys_permissions (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id BIGINT NOT NULL REFERENCES sys_tenants(id) ON DELETE CASCADE,
    role_id BIGINT NOT NULL REFERENCES sys_roles(id) ON DELETE CASCADE,
    collection_id BIGINT NULL,
    actions TEXT[] NOT NULL,
    allowed_fields TEXT[] NOT NULL,
    rls_filter TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS sys_collections (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id BIGINT NOT NULL REFERENCES sys_tenants(id) ON DELETE CASCADE,
    name VARCHAR(64) NOT NULL,
    table_name VARCHAR(64) NOT NULL,
    label VARCHAR(128) NOT NULL,
    is_system BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_collections_tenant_name UNIQUE (tenant_id, name)
);

CREATE TABLE IF NOT EXISTS sys_fields (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    collection_id BIGINT NOT NULL REFERENCES sys_collections(id) ON DELETE CASCADE,
    name VARCHAR(64) NOT NULL,
    type VARCHAR(32) NOT NULL,
    is_required BOOLEAN NOT NULL DEFAULT false,
    is_unique BOOLEAN NOT NULL DEFAULT false,
    is_searchable BOOLEAN NOT NULL DEFAULT false,
    is_encrypted BOOLEAN NOT NULL DEFAULT false,
    default_value TEXT NULL,
    options TEXT[] NULL,
    precision INT NULL,
    scale INT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_fields_collection_name UNIQUE (collection_id, name)
);

CREATE TABLE IF NOT EXISTS sys_jobs (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id BIGINT NOT NULL REFERENCES sys_tenants(id) ON DELETE CASCADE,
    workflow_id BIGINT NULL,
    task_type VARCHAR(64) NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    status VARCHAR(16) NOT NULL DEFAULT 'PENDING',
    run_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    attempts INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 3,
    last_error TEXT NULL,
    locked_by VARCHAR(64) NULL,
    locked_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS sys_audit_logs (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    collection_id BIGINT NOT NULL,
    record_id BIGINT NOT NULL,
    action VARCHAR(16) NOT NULL,
    user_id BIGINT NULL,
    diff JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_sys_jobs_run_at_status ON sys_jobs(run_at, status);
CREATE INDEX IF NOT EXISTS idx_sys_audit_logs_lookup ON sys_audit_logs(collection_id, record_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_sys_users_tenant_id ON sys_users(tenant_id);
CREATE INDEX IF NOT EXISTS idx_sys_roles_tenant_id ON sys_roles(tenant_id);
CREATE INDEX IF NOT EXISTS idx_sys_permissions_tenant_role ON sys_permissions(tenant_id, role_id);
CREATE INDEX IF NOT EXISTS idx_sys_collections_tenant_id ON sys_collections(tenant_id);
CREATE INDEX IF NOT EXISTS idx_sys_fields_collection_id ON sys_fields(collection_id);
CREATE INDEX IF NOT EXISTS idx_sys_audit_logs_tenant_id ON sys_audit_logs(tenant_id);
`

var BaselineMigrations = []Migration{
	{
		Version:     1,
		Description: "initialize core sys_* catalog tables and operational indexes",
		SQL:         BaselineCatalogDDL,
	},
}
