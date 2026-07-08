# Database Migrations

This directory contains the ordered PostgreSQL schema history for Yamata no Orochi. The current schema head is:

```text
0119_convert_bundle_tag_evaluation_ids_to_bigserial.sql
```

There are currently 121 numbered up files and 120 numbered down files. The difference is `0050_remove_short_links_indexes.sql`, which has no matching down migration.

## Naming and Ordering

Most changes use a matching pair:

```text
NNNN_description.sql
NNNN_description_down.sql
```

The history has two duplicate ordinals, so filename—not just the number—is the migration identity:

- `0024_create_sheba_number_on_customers`
- `0024_create_system_company_and_wallet`
- `0104_create_sent_rubika_messages`
- `0104_create_splus_status_results`

New changes should use the next unused ordinal (`0120` after the current head), include a down file whenever rollback is safe, and update both aggregate manifests.

## Current Aggregate-Manifest Issues

`run_all_up.sql` and `run_all_down.sql` are intended as convenience manifests, but the checked-in versions are not fully consistent with the files in this directory.

`run_all_up.sql` currently:

- References missing `0052_add_indexes_to_short_links.sql`; the existing file is `0052_rename_segment_to_level1_and_add_level3.sql`.
- References missing `0054_add_indexes_to_short_link_clicks.sql`; the existing file is `0054_backfill_short_link_clicks_from_short_links.sql`.
- Omits `0104_create_splus_status_results.sql`.

`run_all_down.sql` currently:

- References the corresponding nonexistent `0052_add_indexes_to_short_links_down.sql` and `0054_add_indexes_to_short_link_clicks_down.sql`.
- Omits `0052_rename_segment_to_level1_and_add_level3_down.sql`, `0054_backfill_short_link_clicks_from_short_links_down.sql`, and `0104_create_splus_status_results_down.sql`.
- Includes both `0077_drop_audit_log_customer_fk_down.sql` and `0078_drop_agency_commissions_down.sql` twice.

Do not treat either aggregate file as validated until these entries are reconciled. With `ON_ERROR_STOP=1`, the up manifest stops at the first nonexistent include; without it, `psql` can continue after an error and leave an unexpectedly partial schema.

## Running Migrations

Run commands from the repository root because the manifests use paths such as `migrations/0001_create_account_types.sql`.

After reconciling the aggregate manifest, apply it with fail-fast behavior:

```bash
psql \
  -h "$DB_HOST" \
  -p "$DB_PORT" \
  -U "$DB_USER" \
  -d "$DB_NAME" \
  -v ON_ERROR_STOP=1 \
  -f migrations/run_all_up.sql
```

`make migrate` invokes the same up manifest after checking PostgreSQL connectivity, but the current target does not pass `ON_ERROR_STOP=1`. Prefer the explicit command above when correctness matters.

Apply one migration directly with:

```bash
psql \
  -h "$DB_HOST" \
  -p "$DB_PORT" \
  -U "$DB_USER" \
  -d "$DB_NAME" \
  -v ON_ERROR_STOP=1 \
  -f migrations/0119_convert_bundle_tag_evaluation_ids_to_bigserial.sql
```

These SQL files do not use a migration-state table. Before applying an individual file, verify which predecessors already exist in the target database.

## Rollback

Rollback one change with its exact down file:

```bash
psql \
  -h "$DB_HOST" \
  -p "$DB_PORT" \
  -U "$DB_USER" \
  -d "$DB_NAME" \
  -v ON_ERROR_STOP=1 \
  -f migrations/0119_convert_bundle_tag_evaluation_ids_to_bigserial_down.sql
```

`run_all_down.sql` attempts to remove the entire application schema in reverse order. It is destructive, currently has the manifest issues above, and should not be run against a database containing data that must be retained. Take and verify a backup first.

Migration `0050_remove_short_links_indexes.sql` has no checked-in rollback file. Restoring its removed indexes requires a deliberate replacement migration or manual schema repair based on the preceding schema.

## Migration History by Area

| Range | Main changes |
|---|---|
| `0001`–`0013` | Account types, customers, OTP/session/auth foundations, audit logs, UUIDs, UTC timestamps, and validation changes |
| `0014`–`0020` | Initial SMS campaigns, wallet/accounting, agency commission, payment audit actions, and tax wallet |
| `0021`–`0033` | Agency referrals/discounts, balance snapshots, system identities, transaction types/indexes, admins, line numbers, sessions, and bots |
| `0034`–`0054` | Audience profiles/tags, processed/sent SMS, tickets, short links/clicks/scenarios, crypto payments, job categories, and short-link denormalization |
| `0055`–`0076` | Status jobs/results, campaign statistics, pricing, cancellation, audience cache, multimedia, platform settings, Bale/Soroush Plus delivery, and the generic `campaigns` rename |
| `0077`–`0097` | Legacy FK/table cleanup, deposits/invoices, admin audit actions, base/page prices, ACL requests, permissions, expiry, exports, and refund/invoice audit coverage |
| `0098`–`0106` | Platform-neutral status jobs, tracking IDs, Bale/Soroush Plus/Rubika status data, Rubika sends, campaign test-send auditing, and wallet-charge previews |
| `0107`–`0116` | Bundles, campaign phases, bundle audience selections, audience scores/statistics, normalized scoring, hidden campaigns, and bundle audit actions |
| `0117`–`0119` | Smart-tag evaluation persistence, platform-scoped campaign status jobs, and `BIGSERIAL`/`BIGINT` evaluation identifiers |

## Current Schema Areas

At head, the schema supports:

- Customer, admin, and bot identities, sessions, audit logs, roles, permissions, and maker-checker ACL requests.
- Bundles and multi-platform campaigns with test/execution phases, audience selections, scores, and per-platform sent-message/status data.
- Bundle smart-tag evaluation runs, events, persona attempts, batches, batch attempts, tag snapshots, and score results.
- Wallets, immutable transactions, balance snapshots, fiat payment requests, deposit receipts, invoices, crypto payments, taxes, and agency discounts.
- Audience profiles, tags, segment factors, page/base prices, platform settings, line numbers, short links/clicks, multimedia, and tickets.

## Adding a Migration

1. Choose the next unused four-digit ordinal.
2. Add the up file and a safe down file when possible.
3. Make the preconditions explicit; use schema-qualified names where ambiguity is possible.
4. Add the up include to the end of `run_all_up.sql`.
5. Add the down include to the beginning of `run_all_down.sql`.
6. Test both directions on a disposable database with `ON_ERROR_STOP=1`.
7. Run the application tests affected by the schema change.
8. Update this README when the head, execution behavior, or major schema areas change.

Avoid editing a migration that has already been deployed. Add a corrective migration so every environment retains the same append-only history.
