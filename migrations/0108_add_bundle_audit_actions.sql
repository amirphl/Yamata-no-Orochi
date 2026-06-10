-- Description: Add audit_action_enum values for bundle create operations

ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'bundle_created';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'bundle_creation_failed';
