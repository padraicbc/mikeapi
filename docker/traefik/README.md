# mikeapi + Traefik

This stack runs `mikeapi` behind Traefik so you can add more services later with labels on the same Docker network.

## Local TLS with mkcert

Generate trusted local certs once:

```bash
mkcert -install
mkdir -p /home/padraic/gits/mikeapi/docker/traefik/certs
mkcert \
  -cert-file /home/padraic/gits/mikeapi/docker/traefik/certs/local-cert.pem \
  -key-file /home/padraic/gits/mikeapi/docker/traefik/certs/local-key.pem \
  localhost 127.0.0.1 ::1
```

## Start

```bash
cd /home/padraic/gits/mikeapi
APP_HOST=localhost docker-compose -f docker/docker-compose.traefik.yml up -d --build
```

Then open:

- `https://localhost`
- `http://localhost` (redirects to HTTPS)

## Stop

```bash
docker-compose -f docker/docker-compose.traefik.yml down
```

## Notes

- `mikeapi` is forced to internal HTTP (`DEBUG=true`, `PORT=:9000`).
- Traefik terminates ingress on `:80` and `:443`.
- The app defaults `DB_HOST` to `host.docker.internal` in Docker mode.
- The Docker network uses a fixed subnet by default: `172.29.0.0/24` (override with `MIKEAPI_DOCKER_SUBNET`).
- Traefik dashboard is on `http://localhost:8080` (override with `TRAEFIK_DASHBOARD_PORT`).
- Local TLS certs are read from `docker/traefik/certs/local-cert.pem` and `docker/traefik/certs/local-key.pem`.
- If Docker logs show `client version 1.24 is too old`, set `DOCKER_API_VERSION` in env (default is `1.44` in compose).

## mmrace.app via Cloudflare DNS challenge

When you add your Cloudflare key, use the override file:

```bash
cd /home/padraic/gits/mikeapi
docker-compose --env-file docker/traefik/cloudflare.env \
  -f docker/docker-compose.traefik.yml \
  -f docker/docker-compose.traefik.cloudflare.yml \
  up -d --build
```

Inline env values also work:

```bash
APP_HOST=mmrace.app \
ACME_EMAIL=you@example.com \
CF_DNS_API_TOKEN=your_cloudflare_dns_edit_token \
docker-compose -f docker/docker-compose.traefik.yml -f docker/docker-compose.traefik.cloudflare.yml up -d --build
```

For `www.mmrace.app`, add another router label (or use a host rule with both domains).

## PostgreSQL host access

If `mikeapi` restarts with `no pg_hba.conf entry for host "172.x.x.x"`, allow the fixed Docker subnet:

1. In `pg_hba.conf` add:
   `host    rpdata    padraic    172.29.0.0/24    scram-sha-256`
2. In `postgresql.conf` keep scope tight (no public exposure):
   `listen_addresses = '127.0.0.1,172.29.0.1'`
3. Reload/restart PostgreSQL.

If the `mikeapi_gateway` network already exists from older runs, recreate it once so the fixed subnet applies:

```bash
docker-compose -f docker/docker-compose.traefik.yml down
docker network rm mikeapi_gateway
docker-compose -f docker/docker-compose.traefik.yml up -d --build
```

You can override Docker DB host quickly:

```bash
DB_HOST_DOCKER=your-db-host APP_HOST=localhost docker-compose -f docker/docker-compose.traefik.yml up -d --build
```

If PostgreSQL only allows `hostssl` rules, set `DB_SSLMODE=require` in `.env` (or set `DATABASE_URL` with `sslmode=require`) and restart.

## Add another app later

1. Put the service on the `mikeapi_gateway` network.
2. Add Traefik labels:
   - `traefik.enable=true`
   - `traefik.http.routers.<name>-http.rule=Host(\`your-host\`)`
   - `traefik.http.routers.<name>-http.entrypoints=web`
   - `traefik.http.services.<name>.loadbalancer.server.port=<internal-port>`
