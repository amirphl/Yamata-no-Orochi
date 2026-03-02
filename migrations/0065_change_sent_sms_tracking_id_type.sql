-- Change sent_sms.tracking_id from UUID to string
ALTER TABLE sent_sms
    ALTER COLUMN tracking_id TYPE varchar(64)
    USING tracking_id::text;
