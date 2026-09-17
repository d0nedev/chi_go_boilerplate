DATABASE_URL ?= postgres://postgres:postgres@localhost:55432/chi_product?sslmode=disable
MIGRATE = docker run --rm --network host -v $(CURDIR)/db/migrations:/migrations:ro migrate/migrate:v4.20.1 -path=/migrations -database "$(DATABASE_URL)"

.PHONY: run test test-integration lint vuln sqlc migrate-up migrate-down up down openapi-lint alerts-test loadtest

run:
	go run ./cmd/server

test:
	go test -race ./...

test-integration:
	TEST_DATABASE_URL="$(DATABASE_URL)" go test -race -run Integration -v ./...

lint:
	test -z "$$(gofmt -l cmd internal)"
	go vet ./...

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...

sqlc:
	sqlc generate

migrate-up:
	$(MIGRATE) up

migrate-down:
	$(MIGRATE) down 1

up:
	docker compose up -d --build

down:
	docker compose down

openapi-lint:
	docker run --rm -v $(CURDIR):/spec -w /spec redocly/cli:2.3.0 lint openapi.yaml

alerts-test:
	docker run --rm -v $(CURDIR):/w -w /w --entrypoint promtool prom/prometheus:v3.6.0 check rules prometheus-alerts.yml
	docker run --rm -v $(CURDIR):/w -w /w --entrypoint promtool prom/prometheus:v3.6.0 test rules prometheus-alerts_test.yml

# Needs a running stack with a high RATE_LIMIT_REQUESTS_PER_MINUTE; see docs/runbook.md.
BASE_URL ?= http://localhost:8080
RATE ?= 200
DURATION ?= 60s
loadtest:
	docker run --rm --network host -v $(CURDIR)/loadtest:/scripts:ro -e BASE_URL=$(BASE_URL) -e API_KEY=$(API_KEY) -e RATE=$(RATE) -e DURATION=$(DURATION) grafana/k6:1.3.0 run /scripts/products.js
