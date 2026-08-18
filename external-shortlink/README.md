# External short-link service

This directory is a non-containerized, fully asynchronous Starlette service for
public `GET /{code}` redirects. PostgreSQL remains a separately operated
container. Immutable mappings are cached per Uvicorn worker; click writes use a
short PostgreSQL budget and fall back to a bounded, durable SQLite WAL spool.

The spool assigns every request a UUID idempotency key. PostgreSQL assigns the
monotonic `BIGSERIAL` `click_id` when the event becomes fetchable. A timed-out
insert can therefore be safely spooled: replay uses the UUID constraint and
cannot create a duplicate. Delayed spool events receive IDs only when inserted,
so an ID cursor can never pass and permanently skip them.

## Install

1. Verify the DNS/routing case in the section below before changing production
   traffic.
2. Start the separately managed PostgreSQL container from
   `deploy/postgres.compose.yml`, or point the service at an existing private
   PostgreSQL instance. Do not expose PostgreSQL on a public address.
3. Apply `schema.sql` with `psql` using a migration/operator credential.
4. Create the service account and writable spool directory:

   ```sh
   sudo useradd --system --home /var/lib/external-shortlink --shell /usr/sbin/nologin external-shortlink
   sudo install -d -o external-shortlink -g external-shortlink -m 0750 /var/lib/external-shortlink
   ```

5. Copy this directory to `/opt/external-shortlink`, create a virtualenv, and
   install pinned dependencies:

   ```sh
   cd /opt/external-shortlink
   python3 -m venv .venv
   .venv/bin/pip install --requirement requirements.txt
   ```

6. Install `deploy/external-shortlink.env.example` as
   `/etc/external-shortlink.env` with mode `0600`, replace every secret, and use
   `openssl rand -hex 32` (or stronger) for the API token.
7. Install the systemd unit and Nginx configuration, replace the example domain
   and IP allowlist, obtain a valid TLS certificate, then run:

   ```sh
   sudo systemctl daemon-reload
   sudo systemctl enable --now external-shortlink
   sudo nginx -t
   sudo systemctl reload nginx
   ```

Uvicorn is bound to loopback and accepts forwarded client addresses only from
the local proxy. If the VM has fewer than four useful CPU cores, lower the
worker count in the unit. The SQLite spool is deliberately shared by all
workers and uses WAL/full synchronous durability.

## Production-side configuration

Apply production migration `0135_external_short_link_sync.sql`, then configure:

```dotenv
EXTERNAL_SHORTLINK_ENABLED=true
EXTERNAL_SHORTLINK_BASE_URL=https://links.example.com
EXTERNAL_SHORTLINK_API_TOKEN=the_same_strong_token
EXTERNAL_SHORTLINK_MAPPING_SYNC_INTERVAL=1m
EXTERNAL_SHORTLINK_CLICK_SYNC_INTERVAL=5m
EXTERNAL_SHORTLINK_MAPPING_BATCH_SIZE=5000
EXTERNAL_SHORTLINK_CLICK_PAGE_SIZE=10000
```

The campaign allocation endpoint synchronously uploads newly allocated mappings
and returns codes only after the external service acknowledges the entire batch.
All campaign schedulers release an unprepared campaign back to the approved
queue after an upload failure, so it can retry without sending an unusable link.
A background publisher idempotently backfills older and non-campaign links.

Click synchronization commits `short_link_clicks` inserts and the durable
`external_short_link_sync_state` cursor in one transaction. Its unique
`(source, external_click_id)` index makes retries safe. Acknowledgement is sent
only after commit; acknowledged external rows are retained for the configured
safety window before purge.

Optional mTLS is supported on the Go client with
`EXTERNAL_SHORTLINK_CLIENT_CERT_FILE`, `EXTERNAL_SHORTLINK_CLIENT_KEY_FILE`, and
`EXTERNAL_SHORTLINK_CA_FILE`. Bearer authentication remains mandatory. Restrict
`/api/` at the firewall and Nginx to the production server's fixed egress IP.

## DNS/routing verification

Do not assume moving the origin fixes the domain. Test from at least two probes
outside Iran and one inside Iran before cutover:

```sh
dig +trace jo1n.ir
dig A jo1n.ir @1.1.1.1
dig A jo1n.ir @8.8.8.8
curl -4Iv --connect-timeout 5 https://jo1n.ir/healthz
```

Compare DNS answers and TCP/TLS reachability. If public resolvers return the
expected address but the current Iranian origin times out, move traffic with a
low-TTL DNS cutover, GeoDNS, or a global proxy. If the domain itself does not
resolve reliably or is filtered, use a separate globally reachable short-link
domain; relocating only the backend will not help. Keep the old endpoint during
TTL propagation and test real messaging-network clients before declaring the
cutover complete.

### Observed on 2026-08-18

Four independent Globalping probes in the United States, Germany, Singapore,
and Brazil all resolved `jo1n.ir` successfully to `95.130.241.10`, while HTTPS
requests from all four probes timed out before DNS/TCP/TLS timing could be
reported. From the implementation host, Cloudflare DNS returned the same A
record and HTTPS reached the origin. This is strong evidence for **Case A**:
the domain resolves globally, but the current origin/network path is not
globally reachable. See the public [DNS measurement](https://globalping.io/?measurement=2FetmorCLwPiSeJa100020yHj)
and [HTTPS measurement](https://globalping.io/?measurement=2AdI1suRx9aWLMsHP00020yHi).

Accordingly, the cutover must change the public A record (or place a globally
reachable proxy in front); merely installing this service without changing
traffic routing will not help. The probes are infrastructure vantage points,
not every affected mobile carrier, so the final rollout gate remains a real
short-link test from the affected user networks.

## Monitoring and alerts

Scrape `/metrics` from an allowlisted monitoring host. Recommended alerts:

- any redirect 5xx rate, elevated 404/unknown-code rate, or p95/p99 latency;
- PostgreSQL errors/pool timeouts and readiness failures;
- non-zero spool events, rising spool bytes, oldest spool age, or any rejected
  spool write (capacity rejection is a potential click-loss condition, logged
  at critical severity while redirects remain available);
- mapping upload failures or a stale last-success timestamp;
- stale production click-fetch timestamp, increasing sync lag, or a cursor that
  stops advancing during traffic;
- PostgreSQL disk usage, VM disk/inode usage, TLS expiry, process restarts, and
  external availability from multiple countries.

Nginx access logs provide raw status/latency investigation. PostgreSQL backups,
container health, disk alerts, and restore tests are operated independently.

## Testing and failure drills

Run unit tests and a cache-hot smoke load:

```sh
python3 -m venv .venv
.venv/bin/pip install -r requirements-dev.txt
.venv/bin/pytest
.venv/bin/python tests/bench_redirect.py https://links.example.com/KNOWN_CODE 100 10000
```

Measure cache-hit and cold-cache latency separately. During a controlled drill,
stop only the PostgreSQL container, request an already cached code, confirm a
`302`, verify the spool metrics increase, restart PostgreSQL, and confirm the
spool drains and the click becomes fetchable. Also test a mapping-upload
conflict, a multi-page click backlog, service restart with a non-empty spool,
and a full-spool alert. Never run failure drills first against the only live
node.

## Availability boundary and later HA

Systemd restarts failed processes but this first deployment remains a single
VM/disk/provider/DNS failure domain. A second node can be introduced without
changing the immutable mapping contract: production publishes to both nodes and
imports each node under its own click source/cursor. Do not put two independent
nodes behind one cursor unless their click-ID namespaces are separated.
