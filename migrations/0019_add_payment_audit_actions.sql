-- Migration: Add payment audit actions
-- Up: Add new audit action enum values for payment and wallet operations

-- Add new enum values to audit_action_enum type
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'wallet_charge_initiated';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'wallet_charge_completed';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'wallet_charge_failed';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'wallet_created';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'payment_callback_processed';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'payment_completed';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'payment_failed';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'payment_cancelled';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'payment_expired'; 