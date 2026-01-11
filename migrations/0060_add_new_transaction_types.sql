-- Migration: 0060_add_new_transaction_types.sql
-- Description: Add new transaction types to transaction_type_enum

-- UP MIGRATION
ALTER TYPE transaction_type_enum ADD VALUE IF NOT EXISTS 'credit' AFTER 'adjustment';
ALTER TYPE transaction_type_enum ADD VALUE IF NOT EXISTS 'debit' AFTER 'credit';
ALTER TYPE transaction_type_enum ADD VALUE IF NOT EXISTS 'charge_agency_share_with_tax' AFTER 'debit';
ALTER TYPE transaction_type_enum ADD VALUE IF NOT EXISTS 'discharge_agency_share_with_tax' AFTER 'charge_agency_share_with_tax';
