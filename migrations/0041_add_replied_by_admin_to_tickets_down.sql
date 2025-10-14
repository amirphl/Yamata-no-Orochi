-- Migration: 0041_add_replied_by_admin_to_tickets_down.sql
-- Description: Remove replied_by_admin from tickets

-- DOWN MIGRATION
 
DROP INDEX IF EXISTS idx_tickets_replied_by_admin;
ALTER TABLE tickets DROP COLUMN IF EXISTS replied_by_admin; 