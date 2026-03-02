-- Revert sent_sms.tracking_id back to UUID (non-UUID values become NULL)
ALTER TABLE sent_sms
    ALTER COLUMN tracking_id TYPE uuid
    USING CASE
        WHEN tracking_id ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' THEN tracking_id::uuid
        ELSE NULL
    END;
