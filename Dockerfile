# Stage 1: Build the React + TypeScript frontend
FROM node:22-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm ci || npm install
COPY frontend/ ./
RUN npm run build

# Stage 2: Build the static Go binary with embedded frontend assets
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS go-builder
ARG TARGETOS=linux
ARG TARGETARCH
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY frontend/embed.go ./frontend/embed.go
COPY --from=frontend-builder /app/frontend/dist ./frontend/dist
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o /teepost ./cmd/teepost

# Stage 3: Minimal runtime image
FROM alpine:3.21
RUN apk --no-cache add ca-certificates tzdata wget && \
    adduser -D -u 1000 appuser && \
    mkdir -p /app/data/media && \
    chown -R appuser:appuser /app

WORKDIR /app
COPY --from=go-builder /teepost /app/teepost

USER appuser

ENV PORT=8787 \
    DB_PATH=/app/data/crossposter.db \
    MEDIA_DIR=/app/data/media \
    TZ=UTC

EXPOSE 8787
VOLUME ["/app/data"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8787/healthz >/dev/null || exit 1

ENTRYPOINT ["/app/teepost"]
