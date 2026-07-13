# Smart Targeting tag selection API

All endpoints require customer authentication and verify campaign or bundle ownership.

- `GET /api/v1/bundles/{id}/smart-targeting/tags` provides the paginated table before a campaign exists, so campaign creation can submit selected IDs atomically.
- `GET /api/v1/campaigns/{uuid}/smart-targeting/tags` accepts `search`, `sort_by`, `sort_direction`, `page`, and `page_size` (or `limit`). Supported sort fields are `tag_capacity`, `bundle_persona_fit_score`, `test_phase_avg_ctr`, and `overall_avg_ctr`. The response includes the complete `selected_tag_ids` set and selection summary in addition to the current page.
- `GET /api/v1/campaigns/{uuid}/smart-targeting/selection` returns the complete persisted selection and raw-capacity sum.
- `PUT /api/v1/campaigns/{uuid}/smart-targeting/selection` accepts `{"tag_ids":[1,2]}` and atomically replaces the complete set.
- `POST /api/v1/campaigns/{uuid}/smart-targeting/selection/auto` accepts `count`, `search`, `sort_by`, and `sort_direction`. It selects from the entire filtered order, not the visible page, and replaces the complete set. If `count` exceeds the number of matching available tags, all matching tags are selected. Counts above 10,000 are rejected.

Campaign create/update payloads accept `audience_targeting_method` with these values:

- `smart_targeting`: requires at least one active `selected_tag_ids` value. `level1`, `level2s`, and `level3s` may be omitted or empty and are not used for audience selection.
- `excel`: requires `target_audience_excel_file_uuid`; levels and standard `tags` are not used for audience selection.
- `standard`: uses `level1`, `level2s`, `level3s`, and `tags`.

The field remains optional for backward compatibility. A payload or legacy campaign with an Excel UUID and no method is treated as `excel`; otherwise a missing method is treated as `standard`. Targeting precedence is Smart, Excel, then Standard.

The selectable tag source is resolved per bundle:

- When `current_bundle_tag_scores` contains rows, that complete latest-successful-evaluation snapshot is authoritative. Tag name, display title, audience persona, audience capacity, evaluation run ID, fit score, fit level, relation type, and reason all come from the same snapshot. Later edits to `tags` do not change this view.
- When it contains no rows—even if an evaluation status row exists—the API falls back to active rows in `tags`. Evaluation run ID and score fields are then null.

The two sources are never partially merged. This avoids presenting a mixture of tag metadata captured at different times. Search covers both tag name and display title. An unevaluated bundle uses tag ID ascending as the deterministic order; an evaluated bundle defaults to fit score descending with tag ID ascending as the tie-breaker. CTR fields remain null in this version.

Selection validation, automatic selection, and the snapshots persisted in `campaign_selected_tags` use the same effective source. `selected_raw_capacity` is the sum of those selected tag capacity snapshots. It does not attempt to deduplicate audiences across tags.
