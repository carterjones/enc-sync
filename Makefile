APP_NAME := enc-sync
SRC_FILES := $(wildcard *.go)
GO := go

all: build

build:
	@echo "Building $(APP_NAME)..."
	mkdir -p ./dist
	$(GO) build -o ./dist/$(APP_NAME) $(SRC_FILES)

run-server: build
	@echo "Running $(APP_NAME)..."
	./dist/$(APP_NAME) server server

run-client1: build
	@echo "Running $(APP_NAME)..."
	./dist/$(APP_NAME) client client-1

run-client2: build
	@echo "Running $(APP_NAME)..."
	./dist/$(APP_NAME) client client-2

run-tests: clean-tests run

clean:
	@echo "Cleaning up..."
	rm -f ./dist

clean-tests:
	@echo "Cleaning up test files..."
	rm -rf ./dst-dir

deps:
	@echo "Downloading dependencies..."
	$(GO) mod tidy

.PHONY: all build run clean deps
