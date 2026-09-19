-- Provider-send status, recipient-set consistency, and duplicates
--
-- For a fully executed campaign, missing_send_count, extra_send_count,
-- duplicate_tracking_id_count, and duplicate_phone_count should be zero.
-- Pending rows can be valid while provider status collection is unfinished.

WITH params AS (
    SELECT 955::bigint AS campaign_id
),
campaign_context AS (
    SELECT
        campaign.id AS campaign_id,
        LOWER(BTRIM(campaign.spec->>'platform')) AS platform
    FROM params
    JOIN campaigns AS campaign
      ON campaign.id = params.campaign_id
),
current_processed AS (
    SELECT processed.id
    FROM campaign_context AS context
    JOIN processed_campaigns AS processed
      ON processed.campaign_id = context.campaign_id
     AND processed.is_current
),
expected_phones AS (
    SELECT BTRIM(audience.phone_number) AS phone_number
    FROM campaign_context AS context
    JOIN bundle_audience_selections AS selection
      ON selection.campaign_id = context.campaign_id
    JOIN bundle_audience_selection_members AS member
      ON member.selection_id = selection.id
    LEFT JOIN audience_profiles AS audience
      ON audience.id = member.audience_id
),
all_sends AS (
    SELECT
        'sms'::text AS platform,
        processed_campaign_id,
        phone_number,
        tracking_id::text AS tracking_id,
        status::text AS status,
        parts_delivered
    FROM sent_sms

    UNION ALL

    SELECT
        'bale',
        processed_campaign_id,
        phone_number,
        tracking_id::text,
        status::text,
        parts_delivered
    FROM sent_bale_messages
    WHERE is_current

    UNION ALL

    SELECT
        'rubika',
        processed_campaign_id,
        phone_number,
        tracking_id::text,
        status::text,
        parts_delivered
    FROM sent_rubika_messages

    UNION ALL

    SELECT
        'splus',
        processed_campaign_id,
        phone_number,
        tracking_id::text,
        status::text,
        parts_delivered
    FROM sent_splus_messages
),
campaign_sends AS (
    SELECT sent.*
    FROM campaign_context AS context
    JOIN current_processed AS processed ON TRUE
    JOIN all_sends AS sent
      ON sent.processed_campaign_id = processed.id
     AND sent.platform = context.platform
),
actual_phones AS (
    SELECT BTRIM(phone_number) AS phone_number
    FROM campaign_sends
),
missing AS (
    SELECT phone_number FROM expected_phones
    EXCEPT ALL
    SELECT phone_number FROM actual_phones
),
extra AS (
    SELECT phone_number FROM actual_phones
    EXCEPT ALL
    SELECT phone_number FROM expected_phones
),
duplicate_tracking AS (
    SELECT tracking_id, COUNT(*) AS occurrences
    FROM campaign_sends
    GROUP BY tracking_id
    HAVING COUNT(*) > 1
),
duplicate_phones AS (
    SELECT BTRIM(phone_number) AS phone_number, COUNT(*) AS occurrences
    FROM campaign_sends
    GROUP BY BTRIM(phone_number)
    HAVING COUNT(*) > 1
)
SELECT
    (SELECT COUNT(*) FROM expected_phones) AS expected_phone_count,
    (SELECT COUNT(*) FROM campaign_sends) AS provider_send_count,
    (SELECT COUNT(*) FROM campaign_sends WHERE status = 'successful')
        AS successful_count,
    (SELECT COUNT(*) FROM campaign_sends WHERE status = 'pending')
        AS pending_count,
    (SELECT COUNT(*) FROM campaign_sends WHERE status = 'unsuccessful')
        AS unsuccessful_count,
    (SELECT COUNT(*) FROM campaign_sends
        WHERE tracking_id IS NULL OR BTRIM(tracking_id) = '')
        AS missing_tracking_id_count,
    (SELECT COUNT(*) FROM campaign_sends WHERE parts_delivered < 0)
        AS invalid_parts_delivered_count,
    (SELECT COUNT(*) FROM missing) AS missing_send_count,
    (SELECT COUNT(*) FROM extra) AS extra_send_count,
    COALESCE((
        SELECT SUM(occurrences - 1)
        FROM duplicate_tracking
    ), 0) AS duplicate_tracking_id_count,
    COALESCE((
        SELECT SUM(occurrences - 1)
        FROM duplicate_phones
    ), 0) AS duplicate_phone_count;

