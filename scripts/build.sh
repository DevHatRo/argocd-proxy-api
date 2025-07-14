#!/bin/bash

# Generate Swagger documentation
echo "Generating Swagger documentation..."
go run github.com/swaggo/swag/cmd/swag@latest init

# Build for multiple platforms
echo "Building binaries..."
GOOS=linux GOARCH=amd64 go build -o ./bin/argocd-proxy-linux-amd64 .
GOOS=linux GOARCH=arm64 go build -o ./bin/argocd-proxy-linux-arm64 .
GOOS=darwin GOARCH=arm64 go build -o ./bin/argocd-proxy-darwin-arm64 .

echo "Build completed!"
