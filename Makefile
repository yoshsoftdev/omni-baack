.PHONY: run run-consumer run-scheduler run-all build test lint migrate-up migrate-down migrate-force

# The backend is three processes (6.3): api (HTTP+WS), event-consumer (NATS), scheduler.
run:
	go run ./cmd/api

run-consumer:
	go run ./cmd/event-consumer

run-scheduler:
	go run ./cmd/scheduler

# All three in one terminal for local development; Ctrl+C stops them together.
run-all:
	go build -o bin/ ./cmd/api ./cmd/event-consumer ./cmd/scheduler
	trap 'kill 0' INT TERM; bin/api & bin/event-consumer & bin/scheduler & wait

build:
	go build -ldflags="-s -w" -o bin/ ./cmd/api ./cmd/event-consumer ./cmd/scheduler

test:
	go test ./... -count=1

lint:
	go vet ./...

migrate-up:
	go run ./cmd/migrate -direction=up

migrate-down:
	go run ./cmd/migrate -direction=down

migrate-force:
	@test -n "$(VERSION)" || (echo "usage: make migrate-force VERSION=13" && exit 1)
	go run ./cmd/migrate -direction=force -version=$(VERSION)
