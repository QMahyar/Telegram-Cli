.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -o bin/telegram-cli$(BIN_EXT) ./cmd/telegram-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/telegram-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/telegram-mcp$(BIN_EXT) ./cmd/telegram-mcp

install-mcp:
	go install ./cmd/telegram-mcp

build-all: build build-mcp
