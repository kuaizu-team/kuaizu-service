# Production Docker deployment

The backend must be managed by one Docker Compose project. Do not mix
`docker run` containers with Compose-managed containers that use the same names.

## Database migration gate for this release

Database migrations are deliberately run as a separate pre-deploy gate; the
application workflow must not start a new image against an older schema. Apply
the following files exactly once and in this order:

1. `sql/migration_event_level_summary.sql`
2. `sql/migration_admin_user_website_profile.sql`
3. `sql/20260713_event_manager.sql`
4. `sql/20260715_admin_multi_school_delegation.sql`
5. `sql/20260715_add_welcome_email_delivery.sql`
6. `sql/migration_order_push_status.sql`
7. `sql/20260725_admin_phone.sql`
8. `sql/20260725_collaboration_score_default.sql`
9. `sql/20260725_realtime_collaboration_score.sql`
10. `sql/20260728_collaboration_score_update_notification.sql`

`20260713_event_manager.sql` depends on the `event.school_id` column created by
the first migration. The multi-school migration also assumes the existing
settlement tables are present. The order push status migration is idempotent and
may be safely rerun when deployment history is uncertain. The other ALTER
statements are not rerunnable; if a database was already migrated, run
`sql/20260718_release_preflight.sql` instead
of applying them again. The preflight is read-only and must return no rows.
`20260725_admin_phone.sql` is also not rerunnable; skip it when
`admin_user.phone` already exists. The remaining three collaboration-score
migrations are safe to rerun.

Take a database backup before the first migration. Deploy the application only
after the preflight passes. Do not run
`sql/20260714_event_manager_password.sql` on hash-only deployments. After
deploying the hash-only administrator code, apply
`sql/migration_admin_password_hash_only.sql` only when the obsolete
`password_encrypted` column exists. This migration is intentionally not
rerunnable.

During the same maintenance window, review and manually apply
`sql/migration_user_contact_unique.sql` before reopening user writes, followed
by `sql/migration_operational_indexes.sql` and
`sql/migration_remove_redundant_indexes.sql`. These files are SQL suggestions
and are never executed by the application.

### Fenced message-center deployment gate

The fenced message-center release is a stop-the-world upgrade. A rolling upgrade
or any overlap between the previous and fenced message-center versions is
prohibited, including rollback. The previous worker can delete another owner's
Redis lock and can write promotion/task state without the new epoch checks.

Use this order in the maintenance window:

1. Stop message intake on every previous-version message-center instance and
   wait for all in-flight email and SMS Consumer calls to finish. Confirm that no
   process or container running the previous artifact remains; queued RabbitMQ
   deliveries may stay queued.
2. Inspect Redis keys matching `idempotent:email:promo:*`, recording each exact
   key, value, and TTL. A value of `1` is a legacy ownerless lock. Only after all
   previous workers are stopped, either wait for those exact keys to expire or
   remove each exact key with a compare-and-delete operation that deletes it
   only when its value is still `1`. Never bulk-delete this prefix or delete a
   UUID-valued lock.
3. Manually apply `sql/migration_message_processing_fence.sql`, then run
   `sql/migration_message_processing_fence_verify.sql`. Its single row must
   report `expected_column_count=2`, `passed_column_count=2`, and
   `verification_status=PASS`. The migration is safe to rerun.
4. Start only fenced-version message-center instances. Confirm every instance is
   on the same release before re-enabling message intake, then verify RabbitMQ
   consumption and the promotion reconciliation log.

The field migration is additive. If it was applied before this maintenance
window, do not roll it back; steps 1, 2, and 4 are still mandatory. A rollback to
the previous worker likewise requires stopping and draining every fenced worker
first, so the two implementations never run concurrently.

The latest reviewed export contains six unfinished project promotions that the
first reconciliation pass is expected to settle. Before starting the fenced
workers, run this read-only inventory and obtain business approval for the
current rows:

```sql
SELECT id, order_id, status, total_sent, started_at, created_at,
       processing_epoch, processing_token
FROM email_promotion
WHERE id IN (28, 31, 35, 69, 71, 76)
ORDER BY id;
```

Based on that export, promotions `28`, `31`, `35`, `69`, and `71` are expected
to become `FAILED`, while promotion `76` is expected to become `COMPLETED`.
Re-evaluate this expectation if any returned row has changed since the export;
do not start the worker until the business owner accepts the resulting states.

For the message center, apply `sql/20260718_project_promotion_public_link.sql` from
the `kz-message-center` repository only when the project-promotion template still
contains the legacy Mini Program relative path. It is idempotent; templates
that already use the HTTPS project link require no data update.

## Why the previous deploy failed

`docker compose up -d kuaizu-console` can still try to create `kuaizu-api`
because `kuaizu-console` depends on `kuaizu-api`. If a manually created
container named `kuaizu-api` already exists, Compose cannot create or adopt it,
so it fails with a container name conflict.

## One-time production migration

These steps keep `.env.docker` on the server and do not remove images or data
volumes.

```bash
cd /root/kuaizu-server

docker ps -a --filter "name=^/kuaizu-api$" --filter "name=^/kuaizu-console$"
docker inspect -f '{{.Name}} {{ index .Config.Labels "com.docker.compose.project" }}' kuaizu-api 2>/dev/null || true
docker inspect -f '{{.Name}} {{ index .Config.Labels "com.docker.compose.project" }}' kuaizu-console 2>/dev/null || true

docker stop kuaizu-api kuaizu-console 2>/dev/null || true
docker rm kuaizu-api kuaizu-console 2>/dev/null || true

export VERSION=latest
docker compose up -d --no-build --remove-orphans kuaizu-api kuaizu-console
docker compose ps
```

If the server only has the legacy `docker-compose` command, replace
`docker compose` with `docker-compose`.

## Deploy commands

```bash
cd /root/kuaizu-server
export VERSION=<tag>
gunzip -c kuaizu-server.tar.gz | docker load
docker compose down --remove-orphans
docker compose up -d --no-build --remove-orphans kuaizu-api kuaizu-console
docker compose ps
```

## Health checks

```bash
curl -fsS http://127.0.0.1:8080/health
curl -sS -o /tmp/kuaizu-console-health.out -w '%{http_code}\n' \
  -X POST http://127.0.0.1:8081/admin/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"__healthcheck__","password":"__healthcheck__"}'
docker logs --tail 200 kuaizu-console
```

The admin login health check should return `401` for the fake account. That
means the container, HTTP listener, database lookup, and Nginx upstream path are
alive. Any `5xx` response should fail the deploy.

## Rollback

```bash
cd /root/kuaizu-server
export VERSION=<previous-tag>
docker compose up -d --no-build --remove-orphans kuaizu-api kuaizu-console
docker compose ps
```

## Nginx upstream

Nginx should proxy admin API traffic to the host port, not to a Docker container
IP:

```nginx
location /admin/ {
    proxy_pass http://127.0.0.1:8081/admin/;
}
```

The welcome-email customer-service URL is intentionally outside `/api/v2` so
that opening it never requires JWT authentication. Proxy the exact public path
to the Mini Program API service:

```nginx
location = /open/customer-service {
    proxy_pass http://127.0.0.1:8080/open/customer-service;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

After reloading Nginx, verify that the stable HTTPS entry returns `302` and a
WeChat URL Link in the `Location` header:

```bash
curl -I https://kuaizu.xyz/open/customer-service
```

The destination host should be `wxaurl.cn` or `wxmpurl.cn`. Never put
`WECHAT_SECRET` in Nginx, the database, an email template, or client code; it is
read only by the API container from `.env.docker`.
