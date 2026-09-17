-- Count completely delivered SMS messages sent to a campaign's final audience
--
-- A message is completely delivered only when every provider-reported part is
-- delivered. total_parts > 0 intentionally prevents an incomplete/invalid
-- 0-parts status from being counted as delivered. Set campaign_id below before
-- running.

WITH params AS (
    SELECT 955::bigint AS campaign_id
),
current_processed AS (
    SELECT processed.id, processed.audience_ids
    FROM params
    JOIN processed_campaigns AS processed
      ON processed.campaign_id = params.campaign_id
     AND processed.is_current
),
selected_phones AS (
    SELECT DISTINCT audience.phone_number
    FROM current_processed AS processed
    CROSS JOIN LATERAL UNNEST(processed.audience_ids)
        AS selected(audience_id)
    JOIN audience_profiles AS audience
      ON audience.id = selected.audience_id
    WHERE audience.phone_number IS NOT NULL
)
SELECT COUNT(*) AS completely_delivered_message_count
FROM current_processed AS processed
JOIN sent_sms AS sent
  ON sent.processed_campaign_id = processed.id
JOIN selected_phones AS selected
  ON selected.phone_number = sent.phone_number
JOIN sms_status_results AS status
  ON status.processed_campaign_id = sent.processed_campaign_id
 AND status.tracking_id = sent.tracking_id
WHERE status.total_parts > 0
  AND status.total_delivered_parts = status.total_parts;
