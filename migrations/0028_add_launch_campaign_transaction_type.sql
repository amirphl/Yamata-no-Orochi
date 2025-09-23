-- Migration: 0028_add_launch_campaign_transaction_type.sql
-- Description: Add 'launch_campaign' to transaction_type_enum
 
-- UP MIGRATION
ALTER TYPE transaction_type_enum ADD VALUE IF NOT EXISTS 'launch_campaign' AFTER 'withdrawal'; 