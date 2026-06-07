-- +goose Down
ALTER TABLE admins
    DROP COLUMN IF EXISTS denied_permissions,
    DROP COLUMN IF EXISTS allowed_permissions,
    DROP COLUMN IF EXISTS roles;
