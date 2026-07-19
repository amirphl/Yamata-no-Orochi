-- Create the tag-to-audience-hierarchy reference table used to build the
-- database-backed campaign audience specification.

BEGIN;

CREATE TABLE IF NOT EXISTS src_reference (
    id BIGINT NOT NULL PRIMARY KEY,
    src_address TEXT,
    layer1_category TEXT,
    layer2_category TEXT,
    layer3_category TEXT,
    tag_count BIGINT
);

CREATE INDEX IF NOT EXISTS idx_src_reference_hierarchy
    ON src_reference (
        layer1_category,
        layer2_category,
        layer3_category,
        id
    );

CREATE INDEX IF NOT EXISTS idx_src_reference_layer3
    ON src_reference (layer3_category);

CREATE INDEX IF NOT EXISTS idx_src_layer_all_stats_audience_spec_lookup
    ON src_layer_all_stats (
        layer1_category,
        layer2_category,
        layer3_category,
        calculated_at DESC
    );

COMMIT;
