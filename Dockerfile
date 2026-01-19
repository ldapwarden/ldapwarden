# Build stage for Go backend
FROM golang:1.24-alpine AS backend-builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY cmd/ cmd/
COPY internal/ internal/
COPY db/ db/

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /ldapwarden ./cmd/server

# Build stage for React frontend
FROM node:20-alpine AS frontend-builder

WORKDIR /app

# Copy package files first for better caching
COPY web/package.json web/pnpm-lock.yaml ./
RUN corepack enable && pnpm install --frozen-lockfile

# Copy frontend source
COPY web/ ./

# Build the frontend
RUN pnpm build

# Final stage
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Copy the binary from backend builder
COPY --from=backend-builder /ldapwarden /app/ldapwarden

# Copy the frontend build from frontend builder
COPY --from=frontend-builder /app/dist /app/web/dist

# Copy database migrations
COPY db/migrations /app/db/migrations

# Create non-root user
RUN adduser -D -g '' ldapwarden
USER ldapwarden

# Expose port
EXPOSE 8000

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8000/health || exit 1

# Run the binary
ENTRYPOINT ["/app/ldapwarden"]
