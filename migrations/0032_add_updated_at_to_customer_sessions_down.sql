-- Migration: 0029_add_updated_at_to_customer_sessions_down.sql
-- Description: Remove updated_at column from customer_sessions

-- DOWN MIGRATION
ALTER TABLE customer_sessions
DROP COLUMN IF EXISTS updated_at; 