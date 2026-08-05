.PHONY: build test lint install clean dist npm-pack npm-publish

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
	rm -rf bin/ dist/ vendor/

build-mcp:
	go build -o bin/telegram-mcp$(BIN_EXT) ./cmd/telegram-mcp

install-mcp:
	go install ./cmd/telegram-mcp

build-all: build build-mcp

# dist builds the cross-platform release matrix (scripts/dist.sh) exactly as
# the release workflow does; VERSION overrides the tag used for ldflags.
dist:
	bash scripts/dist.sh $(VERSION)

# npm-pack produces the npm tarball from the repo tree (dry-run by default;
# add REAL=1 to write the .tgz). Version is synced from the git tag:
#   make npm-pack VERSION=v0.1.0
VERSION ?= v0.0.0-dev
npm-pack:
	npm version $(VERSION:v=%) --no-git-tag-version --allow-same-version >/dev/null
	$(if $(REAL),npm pack,npm pack --dry-run)
	npm version 0.0.0-dev --no-git-tag-version --allow-same-version >/dev/null

# npm-publish is the manual fallback when NPM_TOKEN is not wired into CI.
# Requires: npm login (qmahyar) + VERSION=vX.Y.Z matching an existing release.
npm-publish:
	npm version $(VERSION:v=%) --no-git-tag-version --allow-same-version >/dev/null
	npm publish --access public
	npm version 0.0.0-dev --no-git-tag-version --allow-same-version >/dev/null
