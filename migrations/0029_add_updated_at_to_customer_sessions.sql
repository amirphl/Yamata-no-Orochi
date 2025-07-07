-- Migration: 0029_add_updated_at_to_customer_sessions.sql
-- Description: Add updated_at column to customer_sessions to align with model and repository logic

-- UP MIGRATION
ALTER TABLE customer_sessions
ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP;

-- Optional: backfill updated_at with last_accessed_at for existing rows where null
UPDATE customer_sessions
SET updated_at = COALESCE(updated_at, last_accessed_at)
WHERE updated_at IS NULL; 