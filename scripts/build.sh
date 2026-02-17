#!/bin/bash

VERSION=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo "dev")}
BUILD_TIME=$(date -u '+%Y-%m-%dT%H:%M:%SZ')

LDFLAGS="-w -s -X main.Version=$VERSION -X main.BuildTime=$BUILD_TIME"

# Generate Swagger documentation
echo "Generating Swagger documentation..."
go run github.com/swaggo/swag/cmd/swag@latest init

# Create bin directory if it doesn't exist
mkdir -p ./bin

# Build for multiple platforms
# CGO_ENABLED=0 ensures fully static binaries for distroless/static containers
echo "Building binaries (version=$VERSION, build_time=$BUILD_TIME)..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="$LDFLAGS" -o ./bin/argocd-proxy-api-linux-amd64 .
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="$LDFLAGS" -o ./bin/argocd-proxy-api-linux-arm64 .
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="$LDFLAGS" -o ./bin/argocd-proxy-api-darwin-arm64 .

chmod +x ./bin/argocd-proxy-api-*

echo "Build completed!"
