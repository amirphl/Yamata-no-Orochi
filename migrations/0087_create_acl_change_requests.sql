-- +goose Up
CREATE TABLE IF NOT EXISTS acl_change_requests (
    id SERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    target_admin_id BIGINT NOT NULL REFERENCES admins(id),
    requested_by_admin_id BIGINT NOT NULL REFERENCES admins(id),
    approved_by_admin_id BIGINT REFERENCES admins(id),
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    reason TEXT,
    before_roles TEXT[] NOT NULL DEFAULT '{}',
    after_roles TEXT[] NOT NULL DEFAULT '{}',
    before_allowed TEXT[] NOT NULL DEFAULT '{}',
    after_allowed TEXT[] NOT NULL DEFAULT '{}',
    before_denied TEXT[] NOT NULL DEFAULT '{}',
    after_denied TEXT[] NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ,
    applied_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_acl_cr_status ON acl_change_requests(status);
CREATE INDEX IF NOT EXISTS idx_acl_cr_target_admin ON acl_change_requests(target_admin_id);
CREATE INDEX IF NOT EXISTS idx_acl_cr_requester ON acl_change_requests(requested_by_admin_id);

-- +goose Down
DROP TABLE IF EXISTS acl_change_requests;
