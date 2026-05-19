-- Migration: 0079_drop_otp_verifications.sql
-- Description: Remove otp_verifications table (legacy, replaced by Redis OTP storage)

BEGIN;
DROP TABLE IF EXISTS otp_verifications CASCADE;
COMMIT;
