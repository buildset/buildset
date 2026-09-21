BINARY := bin/blog

TEST_PG_CONTAINER := buildset-test-pg
TEST_PG_DATABASE := buildset_test
TEST_PG_PORT := 55432
TEST_PG_DSN := postgres://postgres:postgres@localhost:$(TEST_PG_PORT)/$(TEST_PG_DATABASE)?sslmode=disable

.PHONY: build format lint test test-postgres run

build:
	go build -o $(BINARY) ./cmd/blog

format:
	gofmt -s -w .
	go mod tidy

lint:
	@out="$$(gofmt -s -l .)"; \
	if [ -n "$$out" ]; then echo "gofmt -s needed:"; echo "$$out"; exit 1; fi
	go vet ./...

test:
	go test -v ./...

# The postgres repository tests skip unless TEST_POSTGRES_DSN is set, so `make test` stays offline.
# This starts a throwaway database, runs everything against it, and removes it either way.
test-postgres:
	@docker rm -f $(TEST_PG_CONTAINER) >/dev/null 2>&1 || true
	docker run --rm -d --name $(TEST_PG_CONTAINER) \
		-e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=$(TEST_PG_DATABASE) \
		-e POSTGRES_INITDB_ARGS="--locale=C --encoding=UTF8" \
		-p $(TEST_PG_PORT):5432 postgres:18-alpine
	@echo "waiting for postgres..."
	@for i in $$(seq 1 60); do \
		docker exec $(TEST_PG_CONTAINER) pg_isready -U postgres -d $(TEST_PG_DATABASE) >/dev/null 2>&1 && break; \
		sleep 1; \
	done
	@TEST_POSTGRES_DSN=$(TEST_PG_DSN) go test ./... ; \
		status=$$? ; docker rm -f $(TEST_PG_CONTAINER) >/dev/null ; exit $$status

run: build
	./$(BINARY)
