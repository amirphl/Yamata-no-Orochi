-- +goose Up
-- Description: Add roles/allow/deny permission arrays to admins table for fine-grained authorization
ALTER TABLE admins
    ADD COLUMN IF NOT EXISTS roles TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS allowed_permissions TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS denied_permissions TEXT[] NOT NULL DEFAULT '{}';
