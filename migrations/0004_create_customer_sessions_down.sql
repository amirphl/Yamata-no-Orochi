-- Migration: 0004_create_customer_sessions_down.sql
-- Description: Rollback customer sessions table

-- DOWN MIGRATION
DROP TABLE IF EXISTS customer_sessions CASCADE; 