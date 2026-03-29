-- Allow empty string ip_address by storing as text with inet validation

-- audit_log
DROP INDEX IF EXISTS idx_audit_ip_address;

-- Convert inet to text
ALTER TABLE audit_log
    ALTER COLUMN ip_address TYPE text
    USING CASE
        WHEN ip_address IS NULL THEN NULL
        ELSE ip_address::text
    END;

-- Enforce inet format for non-empty values
ALTER TABLE audit_log
    ADD CONSTRAINT chk_audit_ip_address_inet_or_empty
    CHECK (
        ip_address IS NULL OR
        ip_address = '' OR
        ip_address::inet IS NOT NULL
    );

-- Recreate index, excluding empty strings
CREATE INDEX idx_audit_ip_address ON audit_log(ip_address)
    WHERE ip_address IS NOT NULL AND ip_address <> '';

-- customer_sessions
DROP INDEX IF EXISTS idx_sessions_ip_address;

ALTER TABLE customer_sessions
    ALTER COLUMN ip_address TYPE text
    USING CASE
        WHEN ip_address IS NULL THEN NULL
        ELSE ip_address::text
    END;

ALTER TABLE customer_sessions
    ADD CONSTRAINT chk_sessions_ip_address_inet_or_empty
    CHECK (
        ip_address IS NULL OR
        ip_address = '' OR
        ip_address::inet IS NOT NULL
    );

CREATE INDEX idx_sessions_ip_address ON customer_sessions(ip_address)
    WHERE ip_address IS NOT NULL AND ip_address <> '';
