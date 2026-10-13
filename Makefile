# Run the main server
run:
	go run -race cmd/server/main.go -c config.local
.PHONY: run

# Run tests
test:
	go test -race ./internal/...
.PHONY: test

# Run vet
vet:
	go vet ./internal/...
.PHONY: vet

# Run docker compose
docker:
	docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d --build
.PHONY: docker

# Clean Docker builder cache
clean:
	docker builder prune -f
.PHONY: clean
