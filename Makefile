BINARY := bin/blog

.PHONY: build format lint test run

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

run: build
	./$(BINARY)
