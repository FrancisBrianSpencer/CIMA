# syntax=docker/dockerfile:1
FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o server ./cmd/server

# Production image
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata wget

WORKDIR /app

COPY --from=builder /app/server .

EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=5s --retries=5 --start-period=15s \
  CMD wget --quiet --tries=1 --spider http://localhost:8080/health || exit 1

CMD ["./server"]
