-- Description: Create src_layer_all_stats table

BEGIN;

CREATE TABLE IF NOT EXISTS src_layer_all_stats (
    layer1_category TEXT,
    layer2_category TEXT,
    layer3_category TEXT,
    distinct_users BIGINT,
    calculated_at TIMESTAMPTZ,
    black_users BIGINT,
    white_users BIGINT,
    pink_users BIGINT,
    weak_white BIGINT,
    good_white BIGINT,
    best_white BIGINT,
    weak_black BIGINT,
    good_black BIGINT,
    best_black BIGINT,
    weak_pink BIGINT,
    good_pink BIGINT,
    best_pink BIGINT,
    stat_level TEXT,
    scored_users BIGINT,
    p33 DOUBLE PRECISION,
    p66 DOUBLE PRECISION
);

CREATE INDEX IF NOT EXISTS idx_src_layer_all_stats_p33
    ON src_layer_all_stats (p33);

CREATE INDEX IF NOT EXISTS idx_src_layer_all_stats_p66
    ON src_layer_all_stats (p66);

COMMIT;
