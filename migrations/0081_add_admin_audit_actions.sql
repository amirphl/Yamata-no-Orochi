-- Migration: 0081_add_admin_audit_actions.sql
-- Description: Add audit_action_enum values for admin operations

BEGIN;

ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'admin_list_customers';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'admin_view_customer';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'admin_view_customer_shares';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'admin_view_customer_discounts';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'admin_set_customer_status';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'admin_create_short_links';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'admin_download_short_links';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'admin_download_short_links_with_clicks';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'admin_download_short_links_range';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'admin_download_short_links_by_scenario_regex';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'admin_campaign_approved';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'admin_campaign_rejected';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'admin_campaign_cancelled';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'admin_campaign_rescheduled';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'admin_campaign_list';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'admin_campaign_get';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'admin_remove_audience_spec';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'admin_line_number_create';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'admin_line_number_list';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'admin_line_number_update';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'admin_line_number_report';

COMMIT;
