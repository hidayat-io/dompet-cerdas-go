.PHONY: run build test test-race test-cover lint verify tidy docker-build docker-up docker-down help

# Default target
.DEFAULT_GOAL := help

## run: Run the API server locally
run:
	go run ./cmd/api

## build: Compile the binary
build:
	CGO_ENABLED=0 go build -ldflags="-w -s" -o bin/dompet-cerdas-go ./cmd/api

## test: Run all tests
test:
	go test ./...

## test-race: Run all tests with the race detector
test-race:
	go test -race ./...

## test-cover: Run tests with coverage report
test-cover:
	go test ./... -coverprofile=coverage.txt -covermode=atomic
	go tool cover -html=coverage.txt -o coverage.html
	@echo "Coverage report: coverage.html"

## lint: Run go vet and check formatting
lint:
	go vet ./...
	@test -z "$$(gofmt -l .)" || { echo "Files need gofmt:"; gofmt -l .; exit 1; }

## verify: Run the full gate (lint, build, tests, race)
verify: lint build test test-race
	@echo "All checks passed."

## tidy: Tidy and verify go.mod
tidy:
	go mod tidy
	go mod verify

## docker-build: Build Docker image
docker-build:
	docker compose build

## docker-up: Start services with Docker Compose
docker-up:
	docker compose up -d

## deploy: Cross-compile and deploy to gcp-prau via systemd
deploy:
	./deploy/deploy.sh

## help: Show this help message
help:
	@echo "Available targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'
