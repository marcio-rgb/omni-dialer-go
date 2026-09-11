# Build stage
FROM golang:alpine AS builder
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/dialer-go ./cmd/dialer

# Runtime stage
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=builder /app/dialer-go .

EXPOSE 8080

CMD ["./dialer-go"]
