# Load optional local configuration (see .env.example).
-include .env

DB_URL        ?= postgres://inventory:inventory@localhost:5432/inventory?sslmode=disable
PORT          ?= 8080
ENV           ?= development
MIGRATIONS    := migrations
POSTGRES_USER     ?= inventory
POSTGRES_PASSWORD ?= inventory
POSTGRES_DB       ?= inventory

export DB_URL PORT ENV

SQLC_VERSION    := v1.27.0
MIGRATE_VERSION := v4.17.1

.DEFAULT_GOAL := help
.PHONY: help tools tidy sqlc build run test vet fmt lint check \
        migrate-up migrate-down seed \
        docker-up docker-down docker-seed docker-migrate-down docker-reset

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'

tools: ## Install sqlc and golang-migrate CLIs
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION)
	go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@$(MIGRATE_VERSION)

tidy: ## Resolve dependencies and write go.sum
	go mod tidy

sqlc: ## Generate Go code from SQL (internal/product/productdb)
	sqlc generate

build: ## Build the api and seeder binaries into ./bin
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/api ./cmd/api
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/seeder ./cmd/seeder

run: ## Run the API locally
	go run ./cmd/api

test: ## Run unit and API tests
	go test -race -count=1 ./...

vet: ## Run go vet
	go vet ./...

fmt: ## Format code
	gofmt -s -w .

check: ## Formatting, vet and tests
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed:"; gofmt -l .; exit 1)
	go vet ./...
	go test -race -count=1 ./...

migrate-up: ## Apply all migrations
	migrate -path $(MIGRATIONS) -database "$(DB_URL)" up

migrate-down: ## Roll back the most recent migration
	migrate -path $(MIGRATIONS) -database "$(DB_URL)" down 1

seed: ## Insert sample products (idempotent)
	go run ./cmd/seeder

docker-up: ## Build and start db, migrations and api
	docker compose up --build

docker-down: ## Stop the stack (the database volume is kept)
	docker compose down

docker-seed: ## Seed the dockerized database
	docker compose --profile tools run --rm --build seed

docker-migrate-down: ## Roll back the latest migration in the dockerized database
	docker compose run --rm migrate -path=/migrations \
		-database="postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@db:5432/$(POSTGRES_DB)?sslmode=disable" down 1

docker-reset: ## Stop the stack AND delete the database volume
	docker compose down -v
