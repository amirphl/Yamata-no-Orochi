-- Reconcile campaign-wide audience counts
--
-- For a fully prepared and sent normal-targeting campaign, requested,
-- selection-member, processed-ID, processed-code, and provider-send counts
-- should agree. Smart-targeting attribution rows should be zero.

WITH params AS (
    SELECT 955::bigint AS campaign_id
),
campaign_context AS (
    SELECT
        campaign.id AS campaign_id,
        campaign.num_audience,
        LOWER(BTRIM(campaign.spec->>'platform')) AS platform
    FROM params
    JOIN campaigns AS campaign
      ON campaign.id = params.campaign_id
),
current_processed AS (
    SELECT processed.*
    FROM campaign_context AS context
    JOIN processed_campaigns AS processed
      ON processed.campaign_id = context.campaign_id
     AND processed.is_current
),
send_rows AS (
    SELECT 'sms'::text AS platform, processed_campaign_id
    FROM sent_sms

    UNION ALL

    SELECT 'bale', processed_campaign_id
    FROM sent_bale_messages
    WHERE is_current

    UNION ALL

    SELECT 'rubika', processed_campaign_id
    FROM sent_rubika_messages

    UNION ALL

    SELECT 'splus', processed_campaign_id
    FROM sent_splus_messages
)
SELECT
    context.num_audience AS requested_count,
    (
        SELECT COUNT(*)
        FROM bundle_audience_selections AS selection
        WHERE selection.campaign_id = context.campaign_id
    ) AS selection_record_count,
    COALESCE((
        SELECT SUM(selection.audience_count)
        FROM bundle_audience_selections AS selection
        WHERE selection.campaign_id = context.campaign_id
    ), 0) AS selection_recorded_count,
    (
        SELECT COUNT(*)
        FROM bundle_audience_selections AS selection
        JOIN bundle_audience_selection_members AS member
          ON member.selection_id = selection.id
        WHERE selection.campaign_id = context.campaign_id
    ) AS selection_member_count,
    (
        SELECT COUNT(*)
        FROM campaign_audience_tag_attributions AS attribution
        WHERE attribution.campaign_id = context.campaign_id
    ) AS smart_targeting_attribution_count,
    (SELECT COUNT(*) FROM current_processed)
        AS current_processed_record_count,
    COALESCE((
        SELECT SUM(CARDINALITY(processed.audience_ids))
        FROM current_processed AS processed
    ), 0) AS processed_audience_id_count,
    COALESCE((
        SELECT SUM(CARDINALITY(processed.audience_codes))
        FROM current_processed AS processed
    ), 0) AS processed_audience_code_count,
    (
        SELECT COUNT(*)
        FROM send_rows AS sent
        JOIN current_processed AS processed
          ON processed.id = sent.processed_campaign_id
        WHERE sent.platform = context.platform
    ) AS provider_send_row_count
FROM campaign_context AS context;

