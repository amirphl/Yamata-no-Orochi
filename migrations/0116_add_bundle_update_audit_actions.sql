-- Description: Add audit_action_enum values for bundle update operations

ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'bundle_updated';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'bundle_update_failed';
