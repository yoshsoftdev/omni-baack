.PHONY: run build test lint migrate-up migrate-down

run:
	go run ./cmd/api

build:
	go build -ldflags="-s -w" -o bin/omni-chan-api ./cmd/api

test:
	go test ./... -count=1

lint:
	go vet ./...

migrate-up:
	go run ./cmd/migrate -direction=up

migrate-down:
	go run ./cmd/migrate -direction=down
