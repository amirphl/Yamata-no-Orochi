-- Migration: 0081_add_admin_audit_actions_down.sql
-- Down: Document removal approach for admin audit actions
--
-- PostgreSQL cannot drop individual enum values. To fully remove these values
-- you would need to recreate the enum type and remap the column. This script
-- intentionally leaves the values in place.

-- No-op down migration.
