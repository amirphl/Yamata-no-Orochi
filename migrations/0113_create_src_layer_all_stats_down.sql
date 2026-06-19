-- Description: Drop src_layer_all_stats table

BEGIN;

DROP INDEX IF EXISTS idx_src_layer_all_stats_p66;
DROP INDEX IF EXISTS idx_src_layer_all_stats_p33;

DROP TABLE IF EXISTS src_layer_all_stats;

COMMIT;
