# Ant Project Guide

## Core Mandates
- **MUST**: Run `make fmt` after every code change.

## Project Overview
Ant is a Go microservice. Built with Go, SQLite, Ent ORM, Chi router, JWT auth.

## Technical Stack
- Language: Go v1.26
- Database: SQLite
- ORM: Ent
- Router: chi
- Validation: validator/v10
- Docs: Swagger (swag)
- Logging: log/slog
- Rate Limiting: httprate (primary: 100 req/min per IP; secondary listeners: per-listener config)
- Migrations: Atlas (Ent)

## Directory Structure
```
cmd/api/main.go              Application entry point (primary + secondary servers, -check-config flag)
config/                      YAML configuration
internal/<entity>/           Domain packages (handler, service, repository, model)
internal/platform/http/      Router & middleware (router.go primary, secondary.go listeners)
internal/platform/render/    Standard API responses
internal/db/client.go        SQLite client
ent/schema/                  Ent schema definitions
pkg/config/                  Config loader (viper)
```

## Secondary Listeners
Config-driven extra HTTP servers in the same process (`SECONDARY:` list in config — see README.md for the full reference). Each entry: `NAME`, `ENABLED`, `ADDR` (unique, required), `JWT_SECRET` (optional — listener verifies with this signing key instead of `AUTH.JWT_SECRET`), `RATE_LIMIT` (default 100/1m), `ROUTES` (chi-syntax `"METHOD /path"` allow-list; non-listed = 404).

Key facts:
- Identity ALWAYS comes from JWT — no anonymous mode. Public access = guest tokens: keeper mints short-lived tenant-scoped JWTs (role=guest, signed with `GUEST_JWT_SECRET`) from publishable site keys via `POST /guest-keys/auth`; the `order-intake` listener verifies with that secret, so guest tokens work only there (primary port 401s them, and primary-secret tokens 401 on intake).
- Built by `internal/platform/http/secondary.go` (`NewSecondaryRouter`); reuses the same mount hook/handlers as the primary router — never duplicate handler wiring.
- `/health` + `/metrics` always exposed per listener; swagger only on primary (covers all routes — shared handlers).
- Validation at startup via `pkg/config` `normalizeSecondary()` (addr required/unique, per-entry defaults) + `allowRoutes()` pattern checks; `make config-check` (or `bin/ant -check-config`) vets config without starting servers.
- Env vars cannot override list entries (viper limitation) — YAML only.
- New secondary port → publish it in docker-compose.yml `ports:` (skip for internal s2s listeners — network isolation is the guard).

## Naming Conventions
- Packages: short, lowercase, single-word
- Table names: singular, `ant_` prefix (e.g. `ant_thing`)
- Ent annotation: `entsql.Annotation{Table: "ant_thing"}`
- Env vars: `ANT_` prefix

## Architecture
Handler → Service → Repository (strict direction)

## Workflow
1. `make vendor` — download deps
2. `make generate` — generate ent code
3. `make migrate-gen name=desc` — create migration
4. `make migrate-apply` — apply migration
5. `make fmt` — format (MANDATORY after changes)
6. `make test` — run tests
7. `make swag` — regenerate swagger
8. `make config-check` — validate config (incl. secondary listeners) after config changes
9. `make build && make up` — deploy
10. `make release VERSION=x.y.z` — release: rotates CHANGELOG.md `[Unreleased]` into version section, commits, tags `vx.y.z` (push tags manually)

## Engineering Constraints (mandatory for all new code)

- **Pagination**: every list endpoint MUST accept `limit` (default 50, max 500) and `offset` (default 0) query params and apply them at the query level (`.Limit()/.Offset()`). Never return unbounded result sets.
- **Indexes**: do not add indexes unilaterally. When a query pattern would benefit from one (column in WHERE, JOIN, or ORDER BY), propose it to the user — including composite options where queries filter multiple columns — and add it only after explicit confirmation. Define via `Indexes()` in the ent schema.
- **Transactions**: any operation performing more than one dependent write MUST run inside a single DB transaction (`client.Tx(ctx)`) with rollback on error.
- **Column selection**: when only a subset of columns is needed, use ent `.Select()` instead of fetching full entities.
- **DB portability**: DB driver is configurable (`DATABASE.DRIVER`: sqlite3 | postgres). Keep schema and queries portable across SQLite and Postgres; no driver-specific SQL in business code. Plan: migrate to Postgres as row counts grow.
- **Caching**: frequently-read, rarely-changing responses (e.g. aggregates/stats) should be cached in-memory with a short TTL and explicit invalidation on writes.
- **Sensitive fields**: never expose secrets or password hashes in JSON (`json:"-"`) or logs.
- **Observability**: structured JSON logging via slog (level from `LOG.LEVEL` config); the service exposes Prometheus `/metrics`; new endpoints are automatically covered by the metrics middleware.
- **Outbound HTTP**: any future HTTP client must use a shared client with a timeout sourced from config (never a zero-timeout default client).
- **Locking / race safety**: every operation touching shared mutable state MUST be race-free without sacrificing performance. Guard in-memory state (caches, counters, maps) with `sync.RWMutex` (read locks for reads); protect check-then-write DB flows with a single transaction plus re-check inside it, or a unique constraint. Prefer fine-grained locks over coarse global ones; never hold a lock across I/O. Verify with `go test -race`.
