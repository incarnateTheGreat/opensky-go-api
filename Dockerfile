# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Copy dependency files first (better layer caching)
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server .

# Runtime stage - minimal image
FROM alpine:latest

WORKDIR /app

# Copy binary and data files from builder
COPY --from=builder /app/server .
COPY --from=builder /app/data ./data

# Railway sets PORT env var
EXPOSE 8080

CMD ["./server"]
