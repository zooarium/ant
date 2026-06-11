CUR_DIR := $(notdir $(shell pwd))
export GO_VERSION ?= 1.26.3

# Per-service dev image: build-base + git + pinned tools, baked once (see Dockerfile.dev).
DEV_IMAGE := $(CUR_DIR)-godev:$(GO_VERSION)

# Pinned base image for the final prod stage (kept in sync with Dockerfile by docker-upgrade).
ALPINE_IMAGE := alpine:3.22

# Tool versions sourced from go.mod (single source of truth; goimports ships in x/tools).
TOOLS_VER := $(shell awk '$$1=="golang.org/x/tools"{print $$2; exit}' go.mod)
SWAG_VER  := $(shell awk '$$1=="github.com/swaggo/swag"{print $$2; exit}' go.mod)

# Per-service persistent Go caches (module + build) — survive across ephemeral docker runs.
# Named per service (ant-gomod, ant-gobuild) so each repo keeps its own cache, no cross-service sharing.
GO_CACHE := -v $(CUR_DIR)-gomod:/go/pkg/mod -v $(CUR_DIR)-gobuild:/root/.cache/go-build

# Common Go docker invocation: workspace mount + persistent caches + CGO env, using the prebuilt dev image.
DOCKER_GO := docker run --rm \
	-v $(shell pwd)/..:/workspace -w /workspace/$(CUR_DIR) \
	$(GO_CACHE) \
	-e CGO_ENABLED=1 -e CGO_CFLAGS="-D_LARGEFILE64_SOURCE" \
	$(DEV_IMAGE)

.PHONY: all build up down restart logs ps test benchmark fmt lint swag clean shell help tidy vet generate vendor coverage coverage-view build-local build-prod sql migrate-gen migrate-apply deps-upgrade go-upgrade config-check dev-image sync-tools docker-upgrade

# Build the per-service dev image. Layer-cached: rebuilds only when Dockerfile.dev or GO_VERSION changes.
dev-image:
	@docker build -q -f Dockerfile.dev --build-arg GO_VERSION=$(GO_VERSION) -t $(DEV_IMAGE) . >/dev/null

# Sync Dockerfile.dev tool pins (goimports/swag) to the versions in go.mod.
sync-tools:
	@if [ -z "$(TOOLS_VER)" ] || [ -z "$(SWAG_VER)" ]; then echo "sync-tools: could not read tool versions from go.mod"; exit 1; fi
	sed -i -E 's#(goimports@)v[0-9.]+#\1$(TOOLS_VER)#' Dockerfile.dev
	sed -i -E 's#(swag@)v[0-9.]+#\1$(SWAG_VER)#' Dockerfile.dev
	@echo "sync-tools: goimports@$(TOOLS_VER) swag@$(SWAG_VER)"

# Refresh dockerized toolchain: sync tool pins to go.mod, pull latest base-image patches, rebuild dev image.
docker-upgrade: sync-tools
	docker pull golang:$(GO_VERSION)-alpine
	docker pull $(ALPINE_IMAGE)
	docker pull golangci/golangci-lint:latest
	docker build --pull -f Dockerfile.dev --build-arg GO_VERSION=$(GO_VERSION) -t $(DEV_IMAGE) .
	@echo "docker-upgrade: tool pins synced, base images pulled, dev image rebuilt"

build: vendor
	docker-compose build

up:
	docker-compose up -d

down:
	docker-compose down

restart:
	docker-compose restart

refresh: down swag build up

# Run the full pipeline: format, static analysis, lint, tests, docs, build, deploy
all: fmt vet lint test swag build up

logs:
	docker-compose logs -f

ps:
	docker-compose ps

test: fmt dev-image
	$(DOCKER_GO) go test -mod=vendor -v ./...

benchmark: dev-image
	$(DOCKER_GO) go test -mod=vendor -bench=. -run=^# -benchmem ./...

fmt: dev-image
	$(DOCKER_GO) goimports -w .

lint:
	docker run --rm -v $(shell pwd)/..:/workspace -w /workspace/$(CUR_DIR) \
		$(GO_CACHE) \
		golangci/golangci-lint:latest golangci-lint run -v --modules-download-mode=vendor

swag: dev-image
	$(DOCKER_GO) swag init -g cmd/api/main.go --parseDependency --parseInternal

shell:
	docker-compose exec api sh

tidy: dev-image
	$(DOCKER_GO) go mod tidy

vet: dev-image
	$(DOCKER_GO) go vet -mod=vendor ./...

generate: dev-image
	$(DOCKER_GO) go generate -mod=vendor ./...

vendor: dev-image
	$(DOCKER_GO) sh -c "GOFLAGS=-mod=mod go run entgo.io/ent/cmd/ent generate --feature sql/versioned-migration ./ent/schema && go mod tidy && go mod vendor"

coverage: dev-image
	$(DOCKER_GO) sh -c "go test -mod=vendor -coverprofile=coverage.out ./... && go tool cover -html=coverage.out -o coverage.html"

coverage-view:
	xdg-open coverage.html

build-local:
	go build -o bin/ant ./cmd/api/main.go

build-prod: vendor dev-image
	$(DOCKER_GO) sh -c "go build -mod=vendor -ldflags='-s -w -extldflags \"-static\"' -o bin/ant ./cmd/api/main.go"

deps-upgrade: dev-image
	$(DOCKER_GO) sh -c "go get -u ./... && go mod tidy && go mod vendor"
	$(MAKE) test

go-upgrade:
	@if [ -z "$(version)" ]; then echo "Usage: make go-upgrade version=1.x"; exit 1; fi
	sed -i 's/^go [0-9.]*/go $(version)/' go.mod
	sed -i 's/^export GO_VERSION ?= [0-9.]*/export GO_VERSION ?= $(version)/' Makefile
	sed -i 's/^ARG GO_VERSION=[0-9.]*/ARG GO_VERSION=$(version)/' Dockerfile
	sed -i 's/^ARG GO_VERSION=[0-9.]*/ARG GO_VERSION=$(version)/' Dockerfile.dev
	$(MAKE) build

migrate-gen: dev-image
	$(DOCKER_GO) go run -mod=vendor ent/migrate/main.go $(name)

migrate-apply:
	docker-compose run --rm atlas migrate apply \
		--url "sqlite:///data/ant.db?_fk=1" \
		--dir "file://ent/migrate/migrations" \
		--allow-dirty

sql:
	@if [ -z "$(query)" ]; then echo "Usage: make sql query=\"SQL_QUERY\""; exit 1; fi
	sqlite3 data/ant.db "$(query)"

# Validate config/config.yaml (server, secondary listeners, route patterns) without starting servers
config-check: dev-image
	$(DOCKER_GO) go run -mod=vendor ./cmd/api -check-config

clean:
	docker-compose down --rmi all --volumes --remove-orphans

help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  all            Full pipeline (fmt, vet, lint, test, swag, build, up)"
	@echo "  build          Build Docker images"
	@echo "  up             Start services in background"
	@echo "  down           Stop services"
	@echo "  restart        Restart services"
	@echo "  refresh        Rebuild and restart all services"
	@echo "  logs           Follow container logs"
	@echo "  ps             List running containers"
	@echo "  test           Run unit tests"
	@echo "  benchmark      Run benchmarks"
	@echo "  fmt            Format code (goimports)"
	@echo "  lint           Run linter"
	@echo "  swag           Generate Swagger docs"
	@echo "  tidy           Clean up go.mod"
	@echo "  vet            Run go vet"
	@echo "  generate       Run go generate"
	@echo "  vendor         Create vendor directory"
	@echo "  coverage       Generate test coverage report"
	@echo "  coverage-view  Open test coverage report"
	@echo "  build-local    Build binary locally"
	@echo "  build-prod     Build production binary (static)"
	@echo "  deps-upgrade   Upgrade Go dependencies"
	@echo "  go-upgrade     Upgrade Go version (use version=1.x)"
	@echo "  docker-upgrade Sync tool pins to go.mod + pull latest base images + rebuild dev image"
	@echo "  sync-tools     Sync Dockerfile.dev tool pins (goimports/swag) to go.mod"
	@echo "  migrate-gen    Generate migration (use name=...)"
	@echo "  migrate-apply  Apply pending migrations"
	@echo "  sql            Run SQL query (use query=...)"
	@echo "  config-check   Validate config incl. secondary listeners"
	@echo "  clean          Deep clean containers/images"
	@echo "  help           Show this help"
