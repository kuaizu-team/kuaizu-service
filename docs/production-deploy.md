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
`sql/20260714_event_manager_password.sql` on hash-only deployments.

### Administrator hash-only migration gate

Removing `admin_user.password_encrypted` is a forward-only security migration:
the reversible ciphertext must not be restored after removal. Before the
maintenance window, store one encrypted full backup under least-privilege access,
record its owner, checksum, creation time and deletion deadline, and keep it no
longer than the approved rollback window (maximum seven days). Database and
security administrators are the only roles permitted to access it; application
operators must not extract the ciphertext column.

Use this order:

1. Run `sql/migration_admin_password_hash_only_verify.sql`. The first result must
   report `invalid_hash_count=0` and the second must report
   `verification_status=PRE_MIGRATION`; otherwise stop. Record the current
   administrator count and test-account IDs without exporting hashes or
   ciphertext.
2. Deploy the hash-only backend and admin console while the column still exists.
   Verify login for one platform administrator and one scoped administrator,
   create and delete a temporary administrator, reset a temporary administrator
   password, and edit a temporary event manager. Confirm old and new passwords
   behave as expected and no response exposes a password field.
3. Run `sql/migration_admin_password_hash_only.sql`. It drops the obsolete column
   directly and is safe to rerun; there is no UPDATE/ALTER intermediate state.
4. Run `sql/migration_admin_password_hash_only_verify.sql` again. Its second
   result must be `PASS`, then repeat login, create, reset and event-manager edit
   acceptance against the post-migration schema.

After step 3, normal application rollback to a reversible-password release is
forbidden. If an unrelated emergency requires the previous binary, first stop
all hash-only instances, run
`sql/migration_admin_password_hash_only_compat_rollback.sql` to recreate an empty
nullable compatibility column, then start the previous binary only after the
administrator credential mutation freeze described in [Rollback](#rollback) is
enforced. Do not restore old ciphertext. Existing hashes remain authoritative;
an account that cannot authenticate must remain locked out until the hash-only
release is restored, and its password may be reset only under that hash-only
release. Return to the hash-only release as soon as the unrelated incident is
resolved, prove that the compatibility column is still empty, rerun the migration
and postflight, and securely delete the encrypted backup by its recorded
deadline.

During the same maintenance window, review and manually apply
`sql/migration_user_contact_unique.sql` before reopening user writes, followed
by `sql/migration_operational_indexes.sql` and
`sql/migration_remove_redundant_indexes.sql`. These files are SQL suggestions
and are never executed by the application. Both index migrations are idempotent.
Save the output of `sql/migration_user_contact_unique_verify.sql`,
`sql/migration_message_processing_fence_verify.sql`, and
`sql/migration_operational_indexes_verify.sql`; all verification statuses must be
`PASS`, contact duplicate/normalization counts must be zero, and
`remaining_removed_indexes` must be zero. Then run
`sql/migration_operational_indexes_audit.sql` and attach its `SHOW INDEX` and
`EXPLAIN FORMAT=JSON` result sets to the release record. Production MySQL 8.0.13
does not support `EXPLAIN ANALYZE`; do not substitute an unsupported statement.
If query plans regress, use
`sql/migration_operational_indexes_rollback.sql` during a maintenance window.

### Fenced message-center current baseline and deployment gate

The 2026-08-12 10:06 database export is a post-deployment baseline, not a pending
first deployment. It contains both fence columns and shows promotions `28`, `31`,
`35`, `69`, and `71` as `FAILED` and promotion `76` as `COMPLETED`, all completed
at `2026-08-12 09:54:18`. This proves those reconciliation results were persisted
by that time; the export alone does not prove that the fenced artifact produced
them, which artifact is currently running, or the present Redis and RabbitMQ
state. Run `sql/message_center_runtime_baseline_verify.sql` for every subsequent
release and save its results.

The latest export also contains 20 unfinished legacy rows with null `channel` or
`business_tag`. They predate the current `EMAIL/project_promotion` contract and
are intentionally excluded from the reconciler. Classify them as legacy shadow
records, not active sends. Do not backfill, retry, complete, or delete them under
this deployment procedure; any cleanup requires a separately approved data
migration tied to order/task evidence.

Any future transition from an unfenced worker remains a stop-the-world upgrade.
A rolling upgrade or overlap between unfenced and fenced message-center versions
is prohibited, including rollback. Before each fenced release, independently
confirm the deployed message-center commit is at or after `c4faee5` on every
instance; do not infer this from the database export.

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
3. Apply `sql/migration_message_processing_fence.sql` only if required, then run
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

Before starting or restarting workers, record: every message-center process or
container image digest and commit; RabbitMQ consumer counts and unacked counts for
the email/SMS queues; and exact Redis `idempotent:email:promo:*` key values and
TTLs. UUID-valued keys must correspond to active fenced attempts. Any value `1`,
mixed artifact version, unexpected active promotion, or unexplained unacked
delivery blocks startup until investigated.

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

The admin console authentication storage changes from browser `localStorage`
Bearer tokens to HttpOnly cookies through its BFF. Existing administrators must
sign in once after this release; this expected session invalidation is not an
account or password reset.

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

Do not run the generic rollback command until the target release has been
classified. Record the target tag and one of these two classifications in the
incident log:

- **Hash-only target:** its production administrator repository neither selects
  nor writes `admin_user.password_encrypted`. It may be rolled back directly.
- **Reversible-password target:** any production path selects or writes that
  column. If the hash-only migration has already removed the column, direct
  rollback is prohibited because the old repository will fail SQL operations and
  can recreate reversible ciphertext.

For a hash-only target only, use the normal rollback command:

```bash
cd /root/kuaizu-server
export VERSION=<previous-hash-only-tag>
docker compose up -d --no-build --remove-orphans kuaizu-api kuaizu-console
docker compose ps
```

An emergency rollback to a reversible-password target requires all of the
following controls; omitting any step blocks the rollback:

1. Put administrator credential management into maintenance mode. Stop every
   backend and console instance, then enforce an ingress or application-level
   block on all administrator-account creation and password mutation operations,
   including administrator create/reset/edit and event-manager create/password
   edit. Keep that block in place for the entire old-version interval; an
   operator instruction alone is not a sufficient control.
2. Run `sql/migration_admin_password_hash_only_compat_rollback.sql`. It creates
   only an empty nullable compatibility column. Never restore ciphertext from the
   backup.
3. Start the reversible-password target only for the unrelated emergency. Do not
   create administrators, reset any administrator password, or create/edit event
   manager credentials. Accounts unable to authenticate remain unavailable.
4. As soon as the incident is resolved, stop every reversible-password instance
   before starting a hash-only instance. While the compatibility column exists,
   the following read-only check must return `0`; any nonzero result is a security
   incident and blocks the migration until investigated:

   ```sql
   SELECT COUNT(*) AS reversible_ciphertext_rows
   FROM admin_user
   WHERE password_encrypted IS NOT NULL;
   ```

5. Deploy the hash-only target, run
   `sql/migration_admin_password_hash_only_verify.sql` and require
   `invalid_hash_count=0` plus `verification_status=PRE_MIGRATION`, then run
   `sql/migration_admin_password_hash_only.sql`. Rerun the verifier and require
   `verification_status=PASS` before removing maintenance mode and the credential
   mutation block. Only then may an affected account use password reset.

The `<previous-tag>` command must never be used as a shortcut around this
classification and compatibility gate.

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
