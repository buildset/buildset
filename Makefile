BINARY := bin/blog

COMPOSE_PORT := 8080


.PHONY: build build-all format lint test run compose-up compose-down compose-logs compose-smoke

build:
	go build -o $(BINARY) ./cmd/blog

build-all:
	go build -o bin/ ./cmd/...

format:
	gofmt -s -w .
	go mod tidy

lint:
	@out="$$(gofmt -s -l .)"; \
	if [ -n "$$out" ]; then echo "gofmt -s needed:"; echo "$$out"; exit 1; fi
	go vet ./...

# The postgres repository tests start their own database in a container, so this needs Docker
# running and no other setup.
test:
	go test -v ./...


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
