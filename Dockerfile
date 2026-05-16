# Build stage
FROM golang:1.21.13-alpine AS builder

WORKDIR /app

# Copy all source first (needed for go mod tidy)
COPY . .

# Download dependencies
RUN go mod tidy && go mod download

# Build binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /a2a-platform ./cmd/server

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata curl
ENV TZ=Asia/Shanghai

WORKDIR /app
COPY --from=builder /a2a-platform /app/a2a-platform
COPY etc/config.yaml /app/etc/config.yaml

EXPOSE 18090

HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD curl -f http://localhost:18090/health || exit 1

CMD ["/app/a2a-platform", "-f", "/app/etc/config.yaml"]
