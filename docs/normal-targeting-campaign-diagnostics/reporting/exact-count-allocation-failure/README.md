# Exact-count allocation failure investigation

This query set investigates campaigns `984`, `985`, and `986`, which share a
bundle, tags, grade B, platform, and requested count. Standard bundle selection
requires exactly the requested count before it saves a selection. A failed
`6,696 / 19,500` attempt therefore leaves no allocation, so the next campaign
with the same configuration can observe the same candidates.

Run the files in this order:

1. [`configuration-and-persistence.sql`](configuration-and-persistence.sql)
   confirms matching configurations and whether failed attempts persisted.
2. [`percentile-bound-candidates.sql`](percentile-bound-candidates.sql) lists
   every matching percentile pair.
3. [`audience-reduction-funnel.sql`](audience-reduction-funnel.sql) mirrors the
   standard SMS scheduler predicates for one failed campaign.
4. [`bundle-allocation-exclusions.sql`](bundle-allocation-exclusions.sql) lists
   the bundle allocations excluded by the scheduler.
5. [`allocation-exclusion-attribution.sql`](allocation-exclusion-attribution.sql)
   attributes matching excluded audiences to those allocations.
