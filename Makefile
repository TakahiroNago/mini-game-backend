.DEFAULT_GOAL := help
SHELL := /bin/sh

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

## ---- Go ----
.PHONY: run build fmt fmt-check vet test test-race test-integration lint tidy

run: ## Run the API locally (needs MySQL; reads ./.env if present)
	go run ./cmd/api

build: ## Build binaries into ./bin
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o bin/api ./cmd/api
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/migrate ./cmd/migrate

fmt: ## gofmt all Go files
	gofmt -s -w .

fmt-check: ## Fail if any file is not gofmt-ed
	@out=$$(gofmt -s -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

vet: ## go vet
	go vet ./...

test: ## Unit tests
	go test ./... -count=1

test-race: ## Unit tests with the race detector
	go test ./... -race -count=1

test-integration: ## Integration + concurrency tests against MySQL (uses .env + TEST_DB_NAME)
	TEST_DB_NAME=$${TEST_DB_NAME:-minigame_test} go test ./tests/... -tags=integration -count=1 -v

test-docker: ## Run vet + unit + integration tests inside a Go container (bypasses host antivirus; needs `docker compose up -d mysql`)
	MSYS_NO_PATHCONV=1 docker run --rm --network mini-game-backend_default \
	  -v "$(CURDIR):/src" -v "$$(go env GOMODCACHE):/go/pkg/mod" -v minigame-gobuild:/root/.cache/go-build -w /src \
	  -e DB_HOST=mysql -e DB_PORT=3306 -e TEST_DB_NAME=$${TEST_DB_NAME:-minigame_test} -e GOFLAGS=-buildvcs=false \
	  golang:1.27-alpine sh -c 'go vet ./... && go test ./... -count=1 && go test ./tests/... -tags=integration -count=1'

## ---- Frontend ----
.PHONY: admin-install admin-dev admin-build

admin-install: ## npm ci for the admin UI
	cd frontend/admin && npm ci

admin-dev: ## Run the admin UI dev server (proxies /api to VITE_API_PROXY_TARGET)
	cd frontend/admin && npm run dev

admin-build: ## Type-check and build the admin UI
	cd frontend/admin && npm run build

lint: fmt-check vet ## gofmt + vet

ci: fmt-check vet test ## What the GitHub Actions "go" job runs (minus race + integration)

metrics: ## Print the API's application metrics (API_HOST_PORT from .env, default 8080)
	@port=$$(grep -E '^API_HOST_PORT=' .env 2>/dev/null | cut -d= -f2); curl -s "localhost:$${port:-8080}/metrics" | grep -E '^minigame_'

tidy: ## go mod tidy
	go mod tidy

## ---- Database ----
.PHONY: migrate-up migrate-down migrate-version

migrate-up: ## Apply all migrations
	go run ./cmd/migrate -cmd up

migrate-down: ## Revert the last migration
	go run ./cmd/migrate -cmd down -steps 1

migrate-version: ## Show current schema version
	go run ./cmd/migrate -cmd version

## ---- Docker ----
.PHONY: up down logs ps rebuild

up: ## Start the full stack
	docker compose up -d --build

down: ## Stop the stack (keeps the MySQL volume)
	docker compose down

logs: ## Tail API logs
	docker compose logs -f api

logs-monitoring: ## Tail Prometheus + Grafana logs
	docker compose logs -f prometheus grafana

ps: ## Show container status
	docker compose ps

rebuild: ## Rebuild the API image without cache
	docker compose build --no-cache api

## ---- Misc ----
.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'
