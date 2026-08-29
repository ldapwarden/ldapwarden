# Build stage for Go backend
FROM golang:1.25-alpine AS backend-builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY cmd/ cmd/
COPY internal/ internal/

# Build the binary
# Stamped at link time because the image does not copy .git, so the VCS
# information Go records automatically is not available here. Without it the
# server reports itself as "dev".
ARG VERSION=""
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s -X github.com/ldapwarden/ldapwarden/internal/buildinfo.Version=${VERSION}" \
    -o /ldapwarden ./cmd/server

# Build stage for React frontend
# node:24, not node:20 — pnpm 11 imports the node:sqlite built-in, absent on
# Node 20 (ERR_UNKNOWN_BUILTIN_MODULE), and vite 8 requires Node >=20.19.
FROM node:24-alpine AS frontend-builder

WORKDIR /app

# Copy package files first for better caching
COPY web/package.json web/pnpm-lock.yaml ./
RUN corepack enable && pnpm install --frozen-lockfile

# Copy frontend source
COPY web/ ./

# Build the frontend
RUN pnpm build

# Final stage
FROM alpine:3.24

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Copy the binary from backend builder
COPY --from=backend-builder /ldapwarden /app/ldapwarden

# Copy the frontend build from frontend builder
COPY --from=frontend-builder /app/dist /app/web/dist


# Create non-root user with a fixed UID/GID so volume mounts and
# read-only rootfs policies are predictable across rebuilds. 10001 is
# outside the busybox/alpine system-account range (<1000) and unlikely
# to clash with named host users.
RUN addgroup -g 10001 ldapwarden && \
    adduser -D -u 10001 -G ldapwarden -g '' ldapwarden
USER 10001:10001

# Expose port
EXPOSE 8000

# Liveness, not readiness: docker restarts an unhealthy container, and restarting
# because the directory is briefly unreachable would drop every session for
# nothing. Use /readyz from a load balancer or a readinessProbe instead.
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8000/healthz || exit 1

# Run the binary
ENTRYPOINT ["/app/ldapwarden"]
