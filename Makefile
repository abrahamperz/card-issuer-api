.PHONY: build run test test-race test-coverage lint clean docker-up docker-down migrate

# Build the API binary
build:
	go build -o bin/card-issuer-api ./cmd/server

# Run locally
run: build
	./bin/card-issuer-api

# Run all tests
test:
	go test ./... -v -count=1

# Run tests with race detector
test-race:
	go test ./... -v -race -count=1

# Run tests with coverage
test-coverage:
	go test ./... -v -coverprofile=coverage.out -covermode=atomic
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Lint (requires golangci-lint)
lint:
	golangci-lint run ./...

# Clean build artifacts
clean:
	rm -rf bin/ coverage.out coverage.html

# Docker operations
docker-up:
	docker-compose up --build -d

docker-down:
	docker-compose down -v

# Run migrations manually (requires DATABASE_DSN env var)
migrate:
	@for f in migrations/*.sql; do \
		echo "Running $$f..."; \
		psql "$(DATABASE_DSN)" -f "$$f"; \
	done

# Generate test encryption keys (development only)
generate-keys:
	@echo "ENCRYPTION_KEY=$$(openssl rand -hex 32)"
	@echo "BLIND_INDEX_KEY=$$(openssl rand -hex 32)"
