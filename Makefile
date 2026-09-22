ROOT=$(realpath $(dir $(lastword $(MAKEFILE_LIST))))

BINARY := bin/blog

COMPOSE_PORT := 8080

GO_CMD?=go

GOLANGCI_LINT_CMD?=$(GO_CMD) tool golangci-lint

.DEFAULT_GOAL := .default

.default: format build lint test

.PHONY: help
help: ## Show help
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: build
build:
	$(GO_CMD) build -o bin/ ./cmd/...

.PHONY: format
format:
	$(GO_CMD) fix $(ROOT)/...
	$(GOLANGCI_LINT_CMD) fmt $(ROOT)/...
	$(GO_CMD) mod tidy

.PHONY: lint
lint:
	$(GOLANGCI_LINT_CMD) run $(ROOT)/...

.PHONY: test
test:
	$(GO_CMD) test ./...

.PHONY: run
run: build
	./$(BINARY)

compose-up:
	docker compose up -d --build --wait

compose-down:
	docker compose down

compose-logs:
	docker compose logs -f

# Proves the whole arrangement answers through the one published port.
compose-smoke:
	@set -e; \
	curl -fsS localhost:$(COMPOSE_PORT)/healthz >/dev/null && echo "gateway ok"; \
	code=$$(curl -s -o /dev/null -w '%{http_code}' localhost:$(COMPOSE_PORT)/login); \
	test "$$code" = 200 && echo "identity pages ok ($$code)"; \
	code=$$(curl -s -o /dev/null -w '%{http_code}' localhost:$(COMPOSE_PORT)/); \
	test "$$code" = 303 -o "$$code" = 200 && echo "site ok ($$code)"; \
	code=$$(curl -s -o /dev/null -w '%{http_code}' localhost:$(COMPOSE_PORT)/v1/can); \
	test "$$code" = 404 && echo "internal api not exposed ($$code)"
