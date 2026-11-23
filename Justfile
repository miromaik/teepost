set shell := ["bash", "-euo", "pipefail", "-c"]

# Run locally
dev:
    cd frontend && npm run build
    go run ./cmd/teepost

# Build frontend SPA and stripped static Go binary
build:
    cd frontend && npm install && npm run build
    CGO_ENABLED=0 go build -ldflags="-s -w" -o teepost ./cmd/teepost

# Run all Go tests with race detector and typecheck frontend
test:
    cd frontend && npx tsc --noEmit
    go test -v -race ./tests/... ./...

# Format Go source files
fmt:
    gofmt -w .

# Remove build artifacts
clean:
    rm -rf teepost frontend/dist
