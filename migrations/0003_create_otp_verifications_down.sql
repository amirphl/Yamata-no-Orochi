-- Migration: 0003_create_otp_verifications_down.sql
-- Description: Rollback OTP verifications table

-- DOWN MIGRATION
DROP TABLE IF EXISTS otp_verifications CASCADE;
DROP TYPE IF EXISTS otp_status_enum CASCADE;
DROP TYPE IF EXISTS otp_type_enum CASCADE; 