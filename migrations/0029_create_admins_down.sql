-- Migration: 0029_create_admins_down.sql
-- Description: Rollback admins table

-- DOWN MIGRATION

DROP TABLE IF EXISTS admins CASCADE; 