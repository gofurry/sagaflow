.PHONY: run config-init account-init doctor test web-dev web-build lint build docker-build docker-up docker-down

SAGAFLOW_CONFIG ?=
ACCOUNT_USER ?= admin
ACCOUNT_NAME ?= Creator
ACCOUNT_PASSWORD ?=

run:
	go run ./cmd/sagaflow serve $(if $(SAGAFLOW_CONFIG),--config $(SAGAFLOW_CONFIG),)

config-init:
	go run ./cmd/sagaflow config init $(if $(SAGAFLOW_CONFIG),--output $(SAGAFLOW_CONFIG),)

account-init:
	go run ./cmd/sagaflow account init --username "$(ACCOUNT_USER)" --display-name "$(ACCOUNT_NAME)" --password "$(ACCOUNT_PASSWORD)" $(if $(SAGAFLOW_CONFIG),--config $(SAGAFLOW_CONFIG),)

doctor:
	go run ./cmd/sagaflow doctor $(if $(SAGAFLOW_CONFIG),--config $(SAGAFLOW_CONFIG),)

test:
	go test ./...

web-dev:
	cd web && corepack pnpm dev

web-build:
	cd web && corepack pnpm build

lint:
	cd web && corepack pnpm lint

build: web-build
	go build -o bin/sagaflow ./cmd/sagaflow

docker-build:
	docker compose build

docker-up:
	docker compose up -d

docker-down:
	docker compose down
