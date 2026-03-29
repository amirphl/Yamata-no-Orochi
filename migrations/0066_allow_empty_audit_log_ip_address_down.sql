-- Revert ip_address back to inet (empty strings become NULL)

-- audit_log
DROP INDEX IF EXISTS idx_audit_ip_address;

ALTER TABLE audit_log
    DROP CONSTRAINT IF EXISTS chk_audit_ip_address_inet_or_empty;

ALTER TABLE audit_log
    ALTER COLUMN ip_address TYPE inet
    USING CASE
        WHEN ip_address IS NULL OR ip_address = '' THEN NULL
        ELSE ip_address::inet
    END;

CREATE INDEX idx_audit_ip_address ON audit_log(ip_address)
    WHERE ip_address IS NOT NULL;

-- customer_sessions
DROP INDEX IF EXISTS idx_sessions_ip_address;

ALTER TABLE customer_sessions
    DROP CONSTRAINT IF EXISTS chk_sessions_ip_address_inet_or_empty;

ALTER TABLE customer_sessions
    ALTER COLUMN ip_address TYPE inet
    USING CASE
        WHEN ip_address IS NULL OR ip_address = '' THEN NULL
        ELSE ip_address::inet
    END;

CREATE INDEX idx_sessions_ip_address ON customer_sessions(ip_address)
    WHERE ip_address IS NOT NULL;
