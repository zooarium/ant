# Ant (products & orders service)

A microservice to manage products, attributes and orders.

# Architecture

## Go packages

- `ent` (https://github.com/ent/ent) ORM
- `chi` (https://github.com/go-chi/chi) for routing
- `viper` (https://github.com/spf13/viper) manage multiple environments i.e dev, production configurations
- `slog` go standard library for logging
- `swag` (https://github.com/swaggo/swag) generate RESTful API documentation
- `golangci-lint` (https://github.com/golangci/golangci-lint) linter
- `cors` (https://github.com/go-chi/cors) CORS net/http middleware for Go
- `httprate` (https://github.com/go-chi/httprate) net/http rate limiter middleware
- `validator` (https://github.com/go-playground/validator) field validation

## Directory structure

```
/cmd/api/main.go             Application entry point
/config/
  config.yaml                Base configuration
  config.dev.yaml            Dev overrides
/internal/
  attribute/                 Attribute domain (handler, service, repository, model)
  product/                   Product domain
  order/                     Order domain
  platform/
    http/                    Router, middleware, metrics, secondary listeners
    render/                  Standard API responses
  db/client.go               DB client init (sqlite/postgres)
/ent/schema/                 Ent schema definitions
/pkg/config/                 Configuration loader (viper)
/data/                       SQLite database file (persisted via volume)
/log/                        Application logs (persisted via volume)
```

## Configuration

The application uses `viper`. Loading order:

1. **Defaults**: hardcoded in `pkg/config/config.go`.
2. **Base config**: `config/config.yaml`.
3. **Environment overrides**: `config/config.{ENVIRONMENT}.yaml` (e.g. `config.dev.yaml`).
4. **Environment variables**: `ANT_` prefix, underscore-separated (e.g. `ANT_SERVER_ADDR`).

| Variable | Description | Default |
|----------|-------------|---------|
| `ANT_ENVIRONMENT` | Deployment environment (`dev`, `production`) | `production` |
| `ANT_SERVER_ADDR` | Address the primary server binds to | `:8082` |
| `ANT_SERVER_HOST` | Public host/port used in Swagger docs | `localhost:8082` |
| `ANT_DATABASE_PATH` | Path to the SQLite database file | `data/ant.db` |
| `ANT_DATABASE_DRIVER` | `sqlite3` or `postgres` | `sqlite3` |
| `ANT_DATABASE_DSN` | DSN when driver is `postgres` | `` |
| `ANT_LOG_DIR` | Directory where log files are stored | `log` |
| `ANT_LOG_LEVEL` | `debug`, `info`, `warn`, `error` | `info` |
| `ANT_AUTH_JWT_SECRET` | Shared JWT secret (must match keeper) | — |

## Secondary listeners

Besides the primary server, any number of **secondary listeners** can be
declared in config: extra ports served by the same process, each exposing
only an allow-listed subset of the API, with rate limiting configured per
listener. Identity **always** comes from JWT — there is no anonymous mode.
A per-listener `JWT_SECRET` makes the listener verify with a different
signing key (e.g. keeper's guest secret), so tokens minted for that surface
are cryptographically useless everywhere else.

```yaml
SECONDARY:
  - NAME: "order-intake"         # used in logs; defaults to secondary-<index>
    ENABLED: true                # must be true to start the listener
    ADDR: ":8083"                # required, must be unique across listeners
    # Verify with keeper's guest secret: only guest tokens work here, and
    # guest tokens do NOT work on the primary port (different secret).
    JWT_SECRET: "a-separate-guest-token-secret-key"
    RATE_LIMIT:
      REQUESTS: 60               # default 100
      WINDOW: 1m                 # default 1m
    ROUTES:                      # chi-syntax "METHOD /path" allow-list;
      - "POST /orders"           # anything not listed returns 404
```

Behavior:

- Listeners reuse the same handlers, services and DB client as the primary
  server — no extra process, no duplicate state.
- `/health` and `/metrics` are always exposed on every listener (exempt from
  auth and the allow-list). Swagger is only served on the primary port; it
  documents all routes since the handlers are shared.
- `JWT_SECRET` unset → the listener verifies with the primary
  `AUTH.JWT_SECRET` (same tokens as the main port, smaller surface).
- Config is validated at startup: missing/duplicate `ADDR` or malformed
  `ROUTES` patterns abort boot. Run `make config-check` to vet config
  without starting servers.
- Caveat: environment variables cannot override list entries (viper
  limitation) — secondary listeners are configured via YAML only.
- Docker: publish each secondary port in `docker-compose.yml` (e.g. add
  `- "8083:8083"` under `ports:`).

### Public access via guest tokens (order-intake)

Unauthenticated/public clients never hit ant without a token. Instead they
exchange a **publishable site key** for a short-lived, tenant-scoped guest
JWT at keeper, then call the intake listener with it:

```
browser (shop UI)
  1. POST keeper /guest-keys/auth { "site_key": "gk_..." }
       <- { token (30m), expires_at }          role=guest, signed w/ guest secret
  2. POST ant :8083 /orders   Authorization: Bearer <token>
       -> claims {app_id, division_id, user_id} -> order correctly tenant-scoped
  3. on 401/expiry -> silently re-fetch the guest token
```

- Site keys are managed in keeper (`/guest-keys` CRUD); each binds an app,
  division and a designated guest user. Public by design — forging one only
  grants guest scope for that tenant.
- Guest tokens are signed with `AUTH.GUEST_JWT_SECRET` (keeper) and verified
  here only because `order-intake` sets the matching `JWT_SECRET`. On the
  primary port they fail with 401 — verified by tests.
- Tenant isolation is enforced by claims: a guest order can only reference
  that tenant's products (cross-app product → `product not found`).

### Service-to-service (internal) use

A secondary listener doubles as an internal API port for other zooarium
services — a dedicated allow-listed surface instead of sharing the public
port.

```yaml
SECONDARY:
  - NAME: "internal-s2s"
    ENABLED: true
    ADDR: ":8086"              # do NOT publish in docker-compose ports:
    RATE_LIMIT:
      REQUESTS: 1000           # generous — internal traffic comes from few IPs
      WINDOW: 1m
    ROUTES:
      - "GET /products"
      - "GET /products/{id}"
```

Rules of thumb:

- **Isolation is the guard**: keep the port out of `docker-compose.yml`
  `ports:` — it stays reachable only on the compose network via service DNS
  (`http://ant:8086/products`). On bare metal, bind to a private interface
  (`ADDR: "127.0.0.1:8086"`).
- **Identity**: the calling service forwards the user's JWT (data stays
  scoped per real user), or presents a guest token if the listener is
  configured with the guest `JWT_SECRET`.
- **Rate limit**: internal traffic comes from few caller IPs — raise
  `RATE_LIMIT` well above the public default so legitimate bursts don't
  throttle.
- **Caller side**: per the zooarium constraint, the calling service must use
  a shared HTTP client with a timeout sourced from config (never the
  zero-timeout default client).

## Development

The project uses Docker and a Makefile for development.

- `make all`: Run the full pipeline (fmt, vet, lint, test, swag, build, up).
- `make build`: Build the Docker images.
- `make up` / `make down` / `make restart`: Manage containers.
- `make logs`: Follow the container logs.
- `make fmt`: Format code and organize imports using `goimports` (mandatory after changes).
- `make lint`: Run `golangci-lint`.
- `make vet`: Run `go vet`.
- `make test`: Run all Go tests inside the container.
- `make generate`: Run `go generate` (ent codegen).
- `make vendor`: Create and update the `vendor` directory.
- `make swag`: Generate Swagger documentation.
- `make migrate-gen name=...`: Generate a new versioned migration.
- `make migrate-apply`: Apply pending migrations.
- `make config-check`: Validate `config/config.yaml` — server address, secondary listeners (unique ports, route patterns) — without starting servers.
- `make sql query="..."`: Run a SQL query against the SQLite database.
- `make help`: Display all available commands.

## API Endpoints

All entity routes require a JWT (`Authorization: Bearer <token>`) issued by
the keeper service, except on secondary listeners configured with
`AUTH: false`.

- `GET /health`: Service health (no auth).
- `GET /metrics`: Prometheus metrics (no auth).
- `GET /swagger/*`: Swagger UI (primary port only).
- `POST /attributes`, `GET /attributes`, `GET /attributes/{id}`, `PUT /attributes/{id}`, `DELETE /attributes/{id}`
- `POST /products`, `GET /products`, `GET /products/{id}`, `PUT /products/{id}`, `DELETE /products/{id}`
- `POST /orders`, `GET /orders`, `GET /orders/{id}`, `PUT /orders/{id}`, `DELETE /orders/{id}`, `PATCH /orders/{id}/status`

List endpoints accept `limit` (default 50, max 500) and `offset` (default 0).

## Rate limiting

The primary server is limited to **100 requests per minute per IP**
(`internal/platform/http/router.go`). Each secondary listener has its own
independent limit from its `RATE_LIMIT` config (default 100 req/min).

## Logging & metrics

- Structured JSON logging via `log/slog`; level from `LOG.LEVEL`.
- Logs go to **stdout** and `log/api.log`.
- Prometheus metrics (`http_requests_total`, `http_request_duration_seconds`)
  on `GET /metrics`; all listeners share one registry.

## Deployment checklist

1. **Shared JWT secret**: `ANT_AUTH_JWT_SECRET` must match the keeper service, or all tokens are rejected.
2. **File permissions**: write access to `data/` (SQLite) and `log/`.
3. **CGO/SQLite**: binary must be built with `CGO_ENABLED=1`; Alpine hosts need `sqlite-libs`.
4. **Ports**: open the primary port (8082) plus every enabled secondary listener port in the firewall, and publish them in `docker-compose.yml`.
5. **Validate config**: `make config-check` before rollout.
