# Build frontend
FROM node:22-alpine AS frontend
WORKDIR /app/web/admin
COPY web/admin/package.json web/admin/package-lock.json* ./
RUN npm install
COPY web/admin/ .
RUN npx vite build

# Build Go binary
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -tags frontend -ldflags="-s -w" -o /a2a-platform ./cmd/server

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
