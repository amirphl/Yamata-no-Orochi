-- Migration: 0075_create_sent_splus_messages_down.sql
-- Description: Drop sent_splus_messages table and enum

DROP TABLE IF EXISTS sent_splus_messages;
DROP TYPE IF EXISTS splus_send_status;
