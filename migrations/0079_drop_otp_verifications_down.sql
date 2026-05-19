-- Down Migration: recreate otp_verifications table
BEGIN;
CREATE TABLE IF NOT EXISTS otp_verifications (
    id SERIAL PRIMARY KEY,
    correlation_id UUID DEFAULT gen_random_uuid(),
    customer_id INTEGER NOT NULL,
    otp_code VARCHAR(6) NOT NULL,
    otp_type VARCHAR(20) NOT NULL,
    target_value VARCHAR(255) NOT NULL,
    status VARCHAR(20) NOT NULL,
    attempts_count INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 3,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    ip_address INET,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_otp_customer_id ON otp_verifications(customer_id);
CREATE INDEX IF NOT EXISTS idx_otp_code_target ON otp_verifications(otp_code, target_value);
CREATE INDEX IF NOT EXISTS idx_otp_type_status ON otp_verifications(otp_type, status);
CREATE INDEX IF NOT EXISTS idx_otp_created_at ON otp_verifications(created_at);
CREATE INDEX IF NOT EXISTS idx_otp_expires_at ON otp_verifications(expires_at);
CREATE INDEX IF NOT EXISTS idx_otp_ip_address ON otp_verifications(ip_address) WHERE ip_address IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_otp_customer_type_status ON otp_verifications(customer_id, otp_type, status);
CREATE INDEX IF NOT EXISTS idx_otp_active_codes ON otp_verifications(target_value, otp_type, status) WHERE status = 'pending' AND expires_at > now();
COMMIT;
