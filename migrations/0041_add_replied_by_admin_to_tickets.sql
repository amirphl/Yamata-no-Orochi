-- Migration: 0041_add_replied_by_admin_to_tickets.sql
-- Description: Add replied_by_admin boolean column to tickets

-- UP MIGRATION

ALTER TABLE tickets
ADD COLUMN IF NOT EXISTS replied_by_admin BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_tickets_replied_by_admin ON tickets(replied_by_admin); 