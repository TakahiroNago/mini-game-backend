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

test-integration: ## Integration + concurrency tests (needs TEST_DB_DSN, see README)
	go test ./tests/... -tags=integration -race -count=1 -v

lint: fmt-check vet ## gofmt + vet

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

ps: ## Show container status
	docker compose ps

rebuild: ## Rebuild the API image without cache
	docker compose build --no-cache api

## ---- Misc ----
.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'
