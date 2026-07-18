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
4. `sql/20260714_event_manager_password.sql`
5. `sql/20260715_admin_multi_school_delegation.sql`
6. `sql/20260715_add_welcome_email_delivery.sql`

`20260713_event_manager.sql` depends on the `event.school_id` column created by
the first migration. The multi-school migration also assumes the existing
settlement tables are present. These ALTER statements are not rerunnable; if a
database was already migrated, run `sql/20260718_release_preflight.sql` instead
of applying them again. The preflight is read-only and must return no rows.

Take a database backup before the first migration. Deploy the application only
after the preflight passes, and keep `ADMIN_CREDENTIAL_KEY` configured and
stable across deployments because it encrypts event-manager credentials.

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
