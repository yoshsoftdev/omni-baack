FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY shared/ shared/
COPY services/backend-service/ services/backend-service/
WORKDIR /src/services/backend-service
RUN go mod download && CGO_ENABLED=0 go build -ldflags="-s -w" -o /backend-service ./cmd/api

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /backend-service .
COPY services/backend-service/migrations ./migrations
EXPOSE 8090
CMD ["./backend-service"]
