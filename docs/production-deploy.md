# Production Docker deployment

The backend must be managed by one Docker Compose project. Do not mix
`docker run` containers with Compose-managed containers that use the same names.

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
