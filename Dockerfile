FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY shared/ shared/
COPY services/backend-service/ services/backend-service/
WORKDIR /src/services/backend-service
# One image, three roles (6.3): the command picks the process.
RUN go mod download && \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/backend-service ./cmd/api && \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/event-consumer ./cmd/event-consumer && \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/scheduler ./cmd/scheduler

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/ .
COPY services/backend-service/migrations ./migrations
EXPOSE 8090
CMD ["./backend-service"]
