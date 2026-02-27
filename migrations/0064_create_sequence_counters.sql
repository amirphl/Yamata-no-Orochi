-- Create sequence counters for monotonic IDs
CREATE TABLE IF NOT EXISTS sequence_counters (
    name varchar(64) PRIMARY KEY,
    last_value varchar(64) NOT NULL,
    created_at timestamptz DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'),
    updated_at timestamptz DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
);

INSERT INTO sequence_counters (name, last_value)
VALUES ('sms_tracking_id', repeat('0', 64))
ON CONFLICT (name) DO NOTHING;
