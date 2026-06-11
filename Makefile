CUR_DIR := $(notdir $(shell pwd))
export GO_VERSION ?= 1.26.3

.PHONY: all build up down restart logs ps test benchmark fmt lint swag clean shell help tidy vet generate vendor coverage coverage-view build-local build-prod sql migrate-gen migrate-apply deps-upgrade go-upgrade config-check

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

test: fmt
	docker run --rm -v $(shell pwd)/..:/workspace -w /workspace/$(CUR_DIR) \
		-e CGO_ENABLED=1 \
		-e CGO_CFLAGS="-D_LARGEFILE64_SOURCE" \
		golang:$(GO_VERSION)-alpine \
		sh -c "apk add --no-cache build-base && go test -mod=vendor -v ./..."

benchmark:
	docker run --rm -v $(shell pwd)/..:/workspace -w /workspace/$(CUR_DIR) \
		-e CGO_ENABLED=1 \
		-e CGO_CFLAGS="-D_LARGEFILE64_SOURCE" \
		golang:$(GO_VERSION)-alpine \
		sh -c "apk add --no-cache build-base && go test -mod=vendor -bench=. -run=^# -benchmem ./..."

fmt:
	docker run --rm -v $(shell pwd)/..:/workspace -w /workspace/$(CUR_DIR) golang:$(GO_VERSION)-alpine sh -c "go install golang.org/x/tools/cmd/goimports@latest && goimports -w ."

lint:
	docker run --rm -v $(shell pwd)/..:/workspace -w /workspace/$(CUR_DIR) golangci/golangci-lint:latest golangci-lint run -v --modules-download-mode=vendor

swag:
	docker run --rm -v $(shell pwd)/..:/workspace -w /workspace/$(CUR_DIR) golang:$(GO_VERSION) sh -c "go install github.com/swaggo/swag/cmd/swag@latest && swag init -g cmd/api/main.go --parseDependency --parseInternal"

shell:
	docker-compose exec api sh

tidy:
	docker run --rm -v $(shell pwd)/..:/workspace -w /workspace/$(CUR_DIR) golang:$(GO_VERSION)-alpine sh -c "apk add git && go mod tidy"

vet:
	docker run --rm -v $(shell pwd)/..:/workspace -w /workspace/$(CUR_DIR) golang:$(GO_VERSION)-alpine go vet -mod=vendor ./...

generate:
	docker run --rm -v $(shell pwd)/..:/workspace -w /workspace/$(CUR_DIR) \
		golang:$(GO_VERSION)-alpine \
		go generate -mod=vendor ./...

vendor:
	docker run --rm -v $(shell pwd)/..:/workspace -w /workspace/$(CUR_DIR) golang:$(GO_VERSION)-alpine sh -c "apk add git && GOFLAGS=-mod=mod go run entgo.io/ent/cmd/ent generate --feature sql/versioned-migration ./ent/schema && go mod tidy && go mod vendor"

coverage:
	docker run --rm -v $(shell pwd)/..:/workspace -w /workspace/$(CUR_DIR) \
		-e CGO_ENABLED=1 \
		-e CGO_CFLAGS="-D_LARGEFILE64_SOURCE" \
		golang:$(GO_VERSION)-alpine \
		sh -c "apk add --no-cache build-base && go test -mod=vendor -coverprofile=coverage.out ./... && go tool cover -html=coverage.out -o coverage.html"

coverage-view:
	xdg-open coverage.html

build-local:
	go build -o bin/ant ./cmd/api/main.go

build-prod: vendor
	docker run --rm -v $(shell pwd)/..:/workspace -w /workspace/$(CUR_DIR) \
		-e CGO_ENABLED=1 \
		-e CGO_CFLAGS="-D_LARGEFILE64_SOURCE" \
		golang:$(GO_VERSION)-alpine \
		sh -c "apk add --no-cache build-base && go build -mod=vendor -ldflags='-s -w -extldflags \"-static\"' -o bin/ant ./cmd/api/main.go"

deps-upgrade:
	docker run --rm -v $(shell pwd)/..:/workspace -w /workspace/$(CUR_DIR) \
		golang:$(GO_VERSION)-alpine \
		sh -c "apk add git && go get -u ./... && go mod tidy && go mod vendor"
	$(MAKE) test

go-upgrade:
	@if [ -z "$(version)" ]; then echo "Usage: make go-upgrade version=1.x"; exit 1; fi
	sed -i 's/^go [0-9.]*/go $(version)/' go.mod
	sed -i 's/^export GO_VERSION ?= [0-9.]*/export GO_VERSION ?= $(version)/' Makefile
	sed -i 's/^ARG GO_VERSION=[0-9.]*/ARG GO_VERSION=$(version)/' Dockerfile
	$(MAKE) build

migrate-gen:
	docker run --rm -v $(shell pwd)/..:/workspace -w /workspace/$(CUR_DIR) \
		-e CGO_ENABLED=1 \
		-e CGO_CFLAGS="-D_LARGEFILE64_SOURCE" \
		golang:$(GO_VERSION)-alpine \
		sh -c "apk add --no-cache build-base && go run -mod=vendor ent/migrate/main.go $(name)"

migrate-apply:
	docker-compose run --rm atlas migrate apply \
		--url "sqlite:///data/ant.db?_fk=1" \
		--dir "file://ent/migrate/migrations" \
		--allow-dirty

sql:
	@if [ -z "$(query)" ]; then echo "Usage: make sql query=\"SQL_QUERY\""; exit 1; fi
	sqlite3 data/ant.db "$(query)"

# Validate config/config.yaml (server, secondary listeners, route patterns) without starting servers
config-check:
	docker run --rm -v $(shell pwd)/..:/workspace -w /workspace/$(CUR_DIR) \
		-e CGO_ENABLED=1 \
		-e CGO_CFLAGS="-D_LARGEFILE64_SOURCE" \
		golang:$(GO_VERSION)-alpine \
		sh -c "apk add --no-cache build-base && go run -mod=vendor ./cmd/api -check-config"

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
	@echo "  migrate-gen    Generate migration (use name=...)"
	@echo "  migrate-apply  Apply pending migrations"
	@echo "  sql            Run SQL query (use query=...)"
	@echo "  config-check   Validate config incl. secondary listeners"
	@echo "  clean          Deep clean containers/images"
	@echo "  help           Show this help"
