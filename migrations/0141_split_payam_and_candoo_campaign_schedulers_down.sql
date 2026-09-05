BEGIN;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM payam_processed_campaigns)
       OR EXISTS (SELECT 1 FROM candoo_processed_campaigns)
       OR EXISTS (SELECT 1 FROM payam_sent_sms)
       OR EXISTS (SELECT 1 FROM candoo_sent_sms) THEN
        RAISE EXCEPTION 'cannot roll back scheduler split while provider-owned runtime data exists';
    END IF;
END $$;

DROP TABLE IF EXISTS candoo_sms_send_attempts;
DROP TABLE IF EXISTS payam_sms_send_attempts;
DROP TABLE IF EXISTS candoo_sms_status_results;
DROP TABLE IF EXISTS payam_sms_status_results;
DROP TABLE IF EXISTS candoo_sms_status_jobs;
DROP TABLE IF EXISTS payam_sms_status_jobs;
DROP TABLE IF EXISTS candoo_sent_sms;
DROP TABLE IF EXISTS payam_sent_sms;
DROP TABLE IF EXISTS candoo_processed_campaigns;
DROP TABLE IF EXISTS payam_processed_campaigns;
DROP SEQUENCE IF EXISTS candoo_scheduler_customer_id_seq;

ALTER TABLE line_numbers ALTER COLUMN provider SET DEFAULT 'payamsms';

COMMIT;
