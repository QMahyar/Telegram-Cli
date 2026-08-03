.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -o bin/telegram-pp-cli$(BIN_EXT) ./cmd/telegram-pp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/telegram-pp-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/telegram-pp-mcp$(BIN_EXT) ./cmd/telegram-pp-mcp

install-mcp:
	go install ./cmd/telegram-pp-mcp

build-all: build build-mcp
