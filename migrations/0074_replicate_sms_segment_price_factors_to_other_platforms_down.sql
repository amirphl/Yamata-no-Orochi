-- Migration: 0074_replicate_sms_segment_price_factors_to_other_platforms_down.sql
-- Description: No-op rollback for data backfill migration 0074 (non-reversible)

-- This migration only inserts missing rows derived from existing SMS data.
-- Rolling it back safely is not deterministic, so this down migration is intentionally a no-op.
