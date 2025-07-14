#!/bin/bash

# Generate Swagger documentation
echo "Generating Swagger documentation..."
go run github.com/swaggo/swag/cmd/swag@latest init

# Create bin directory if it doesn't exist
mkdir -p ./bin

# Build for multiple platforms
echo "Building binaries..."
GOOS=linux GOARCH=amd64 go build -o ./bin/argocd-proxy-api-linux-amd64 .
GOOS=linux GOARCH=arm64 go build -o ./bin/argocd-proxy-api-linux-arm64 .
GOOS=darwin GOARCH=arm64 go build -o ./bin/argocd-proxy-api-darwin-arm64 .

chmod +x ./bin/argocd-proxy-api-*

echo "Build completed!"
