# Lor build. Node is needed only at build time.
GO      ?= go
PNPM    ?= pnpm
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/joshualent/lor/internal/version.Version=$(VERSION)
BIN     := bin

.PHONY: all generate web web-deps build test test-go test-web e2e screens integration lint dev clean

all: build

## generate: OpenAPI → Go (oapi-codegen) and TypeScript (openapi-typescript). Output is committed.
generate:
	$(GO) tool oapi-codegen -config api/oapi-admin.yaml api/admin.yaml
	$(GO) tool oapi-codegen -config api/oapi-agent.yaml api/agent.yaml
	$(GO) tool oapi-codegen -config api/oapi-client.yaml api/agent.yaml
	cd web && $(PNPM) exec openapi-typescript ../api/admin.yaml -o src/lib/api/admin.d.ts
	cd web && $(PNPM) exec prettier --write src/lib/api/admin.d.ts >/dev/null

web-deps:
	$(PNPM) -C web install --frozen-lockfile

## web: build the SPA into web/build (embedded by lor-server).
web: web-deps
	$(PNPM) -C web build

## build: both static binaries with the SPA embedded.
build: web
	@mkdir -p $(BIN)
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN)/ ./cmd/...

## test: everything that doesn't need Docker.
test: test-go test-web

test-go: web
	$(GO) vet ./...
	$(GO) test ./...

test-web: web-deps
	$(PNPM) -C web run check
	$(PNPM) -C web run lint
	$(PNPM) -C web exec vitest run

## e2e: Playwright against lor-server --dev (needs: make build; pnpm -C web exec playwright install chromium).
e2e: build
	$(PNPM) -C web exec playwright test

## screens: seed realistic data and screenshot every page (desktop + phone) into SCREENS_DIR. Not in CI.
SCREENS_DIR ?= test/screens/.tmp/out
screens: build
	$(GO) build -o $(BIN)/fakessh ./test/e2e/fakessh
	LOR_SCREENS_DIR=$(abspath $(SCREENS_DIR)) $(PNPM) -C web exec playwright test -c playwright.screens.config.ts

## integration: real OpenSSH in Docker (DISTRO=debian:12 by default).
integration: build
	$(GO) test -tags integration -count=1 -timeout 30m -v ./test/integration/...

## dev: lor-server --dev on 127.0.0.1:8444 plus the Vite dev server (hot reload) on :5173.
dev: web
	@mkdir -p $(BIN)
	$(GO) build -o $(BIN)/lor-server ./cmd/lor-server
	@trap 'kill 0' INT TERM EXIT; \
	./$(BIN)/lor-server serve --dev --dev-data-dir .dev-data & \
	$(PNPM) -C web dev

clean:
	rm -rf $(BIN) web/build web/.svelte-kit
