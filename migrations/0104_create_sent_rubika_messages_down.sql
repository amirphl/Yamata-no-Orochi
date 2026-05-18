-- Migration: 0104_create_sent_rubika_messages_down.sql
-- Description: Drop sent_rubika_messages table

DROP TABLE IF EXISTS sent_rubika_messages;
DROP TYPE IF EXISTS rubika_send_status;
