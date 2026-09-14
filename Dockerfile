# Build stage
FROM golang:bookworm AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/dialer-go ./cmd/dialer

# Runtime stage
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /app/dialer-go .
COPY bin/ ./bin/
COPY models/ ./models/
COPY amd.conf ./amd.conf

EXPOSE 8080

CMD ["./dialer-go"]
