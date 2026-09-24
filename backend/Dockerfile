FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
COPY . .

# Go and dependency resolution happen inside Docker; Go is not required on the host.
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/cima-api ./cmd/server

FROM alpine:3.20

WORKDIR /app
RUN adduser -D -H appuser
COPY --from=builder /out/cima-api /app/cima-api
USER appuser

EXPOSE 8080
CMD ["/app/cima-api"]
