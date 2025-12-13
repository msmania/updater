## Makefile for the updater Go project

# Binary name
BINARY_NAME := updater

# Directories
CMD_DIR := ./cmd/main
BIN_DIR := ./bin

VERSION ?= $(shell git describe --tags --abbrev=0 2>/dev/null || echo dev)

GOFLAGS := -ldflags="-s -w -X main.version=$(VERSION)"

.PHONY: all build run clean test

all: build

# Build the binary
build:
	@mkdir -p $(BIN_DIR)
	go build $(GOFLAGS) -o $(BIN_DIR)/$(BINARY_NAME) $(CMD_DIR)

# Run the application directly (without building a binary)
run:
	go run $(CMD_DIR)/main.go

# Clean generated files
clean:
	rm -rf $(BIN_DIR)

# Run tests (if any)
test:
	go test ./...
