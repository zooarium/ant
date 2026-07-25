# Production Environment Setup — Docker (ant)

This is the Docker-based production path (build the image, run via
`docker-compose.yml`).

## 1. Server sizing

See `keeper/docs/HARDWARE_REQUIREMENTS.md` (sibling repo) for the full
measured breakdown — covers keeper + squirrel + ant together, since they
typically share one small box.

- Absolute floor (all 3 services + nginx): 1 vCPU / 1GB RAM / 5GB disk.
- Recommended: 2 vCPU / 2GB RAM / 10GB disk.

## 2. Install Docker

Install Docker Engine + the Compose plugin via your distro's package manager
or Docker's official install instructions (e.g. `docker.io`/`docker-ce` +
`docker-compose-plugin`). Confirm with:

```bash
docker compose version
```

## 3. Deploy the repo

```bash
git clone <ant-repo> /opt/ant   # or copy the working tree
cd /opt/ant
```

`vendor/` is checked in — no `go mod download` needed on the prod box. Build
on a separate CI/dev machine and ship only the final image if the prod box
is resource-constrained — the builder stage pulls `golang:alpine` (~241MB)
plus `build-base`, which is wasted disk/CPU on a small server.

## 4. Inject real secrets

`config/config.yaml` ships with **placeholder** secrets only — production
must override via env (viper prefix `ANT_`, `.` → `_`). Required:

| Config key | Env var | Notes |
|---|---|---|
| `AUTH.JWT_SECRET` | `ANT_AUTH_JWT_SECRET` | primary JWT signing key |

Optional, depending on which features are turned on:

| Config key | Env var | Notes |
|---|---|---|
| `IMPERSONATION.ENABLED` | `ANT_IMPERSONATION_ENABLED` | off by default |
| `IMPERSONATION.JWT_SECRET` | `ANT_IMPERSONATION_JWT_SECRET` | must match keeper's `AUTH.IMPERSONATION_JWT_SECRET` |
| `IMPERSONATION.KEEPER_BASE_URL` | `ANT_IMPERSONATION_KEEPER_BASE_URL` | reachable keeper base URL |
| `CAPTCHA.SECRET` | `ANT_CAPTCHA_SECRET` | required if `CAPTCHA.ENABLED=true` (reCAPTCHA v3 on public order routes) |
| `SECONDARY[order-intake].JWT_SECRET` | — (YAML only, env can't override list entries) | must match keeper's `AUTH.GUEST_JWT_SECRET` |

Set these in `docker-compose.yml`'s `environment:` block (or an untracked
`.env`/override file) — never commit real secrets.

## 5. Reverse proxy

Put nginx (or equivalent) in front for TLS termination and routing to the
container's published ports — primary (`8082`) and the `order-intake`
secondary listener (`8083`), which is public-facing and must be reachable
for storefront/guest traffic.

## 6. Build and run

```bash
make build && make up
```

Compose already sets `restart: always`, `mem_limit: 256m`, `cpus: "1.0"`,
and `json-file` log rotation (10MB × 3) on the `api` service.

## 7. Log rotation for the bind-mounted file

Docker's log driver only rotates its own stdout copy. The bind-mounted
`./log/api.log` needs the host-level `logrotate.conf` shipped in this repo:

```bash
cp logrotate.conf /etc/logrotate.d/ant-api
```

Adjust the path glob inside first to match the real deployment directory.
See `docs/LOGGING.md` for the full picture (why two log copies exist, why
`copytruncate`).

## 8. Verify

```bash
curl http://localhost:8082/health
curl http://localhost:8083/health
docker compose ps
docker stats --no-stream
```

## 9. Updating

```bash
git pull
make vendor   # if dependencies changed
make build && make up
```

Auto-migration runs on startup — no separate migration step needed for
routine updates.
