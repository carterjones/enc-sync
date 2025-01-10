# Makefile for go-crdt-ipfs-sync

APP_NAME := go-crdt-ipfs-sync
SRC_FILES := $(wildcard *.go)
GO := go

# Default target
all: build

# Build the application
build:
	@echo "Building $(APP_NAME)..."
	mkdir -p ./dist
	$(GO) build -o ./dist/$(APP_NAME) $(SRC_FILES)

# Run the application
run: build
	@echo "Running $(APP_NAME)..."
	./dist/$(APP_NAME)

# Clean up generated files
clean:
	@echo "Cleaning up..."
	rm -f ./dist

# Download and tidy up dependencies
deps:
	@echo "Downloading dependencies..."
	$(GO) mod tidy

.PHONY: all build run clean deps
