# Athlix backend. Run from backend/.
# Local: `docker compose up -d` in the repo root, then: make dev-secrets migrate run
SHELL := /bin/bash
export ENV ?= local
export DB_DSN ?= athlix:athlix-local@tcp(127.0.0.1:3306)/athlix?parseTime=true&loc=UTC&multiStatements=true
export REDIS_ADDR ?= 127.0.0.1:6379
export DEV_FIXED_OTP ?= 123456
LAMBDAS := api ws worker migrate

.PHONY: run worker-job test cover lint fmt migrate dev-secrets create-admin import-districts build-lambda clean

run: ## start the API (+ local WebSocket, in-process events) on :8080
	go run ./cmd/api

worker-job: ## run one scheduled job locally: make worker-job JOB=event.complete_past
	go run ./cmd/worker -job $(JOB)

test: ## unit tests (no MySQL/Redis needed: sqlmock + miniredis)
	go test ./...

cover:
	go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out | tail -1

lint: ## gofmt + vet (CI runs the same)
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	go vet ./...

fmt:
	gofmt -w .

migrate: ## apply backend/migrations to DB_DSN
	go run ./cmd/migrate

dev-secrets: ## write .secrets.local.json with random dev keys (gitignored)
	go run ./cmd/devsecrets

create-admin: ## make create-admin EMAIL=you@example.com NAME="You" ROLE=super_admin
	go run ./cmd/admincli create-admin -email "$(EMAIL)" -name "$(NAME)" -role "$(or $(ROLE),super_admin)"

import-districts: ## upsert districts from an LGD CSV: make import-districts FILE=lgd.csv
	go run ./cmd/admincli import-districts -file "$(FILE)"

build-lambda: ## build arm64 Lambda zips into dist/<name>.zip
	@mkdir -p dist
	@for f in $(LAMBDAS); do \
	  echo "building $$f"; \
	  GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -tags lambda.norpc -o dist/$$f/bootstrap ./cmd/$$f || exit 1; \
	  (cd dist/$$f && zip -q -FS ../$$f.zip bootstrap) || exit 1; \
	done
	@ls -la dist/*.zip

clean: ## build output lives in dist/ and coverage.out (both gitignored); delete them by hand
	@ls -d dist coverage.out 2>/dev/null || echo "nothing to clean"
