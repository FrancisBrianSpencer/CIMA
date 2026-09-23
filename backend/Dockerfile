FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/cima-api ./cmd/server
FROM alpine:3.20
COPY --from=builder /out/cima-api /cima-api
EXPOSE 8080
CMD ["/cima-api"]
