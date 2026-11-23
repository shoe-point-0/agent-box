.PHONY: help fmt fmt-check lint deps test test-race vet build install smoke automation-check check clean

GO ?= go
BIN_DIR ?= bin
INSTALL_DIR ?= $(HOME)/.local/bin
TOOLS_DIR ?= .tools/bin
GOLANGCI_LINT_VERSION := v2.13.1
GOLANGCI_LINT := $(TOOLS_DIR)/golangci-lint

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || printf 'devel')
COMMIT ?= $(shell git rev-parse --verify HEAD 2>/dev/null || printf 'unknown')
BUILD_DATE ?= $(shell git show -s --format=%cI HEAD 2>/dev/null || printf 'unknown')
VERSION_PACKAGE := github.com/shoe-point-0/agent-box/internal/version
LDFLAGS := -X $(VERSION_PACKAGE).Version=$(VERSION) -X $(VERSION_PACKAGE).Commit=$(COMMIT) -X $(VERSION_PACKAGE).BuildDate=$(BUILD_DATE)

help:
	@printf '%s\n' \
		'make check      run the complete local quality gate' \
		'make build      build bin/abox and bin/aboxd with version metadata' \
		'make install    install abox and aboxd to ~/.local/bin (override INSTALL_DIR)' \
		'make fmt        format Go source' \
		'make test       run unit tests' \
		'make test-race  run tests with the race detector' \
		'make lint       run pinned golangci-lint' \
		'make clean      remove generated binaries and local tools'

fmt:
	$(GO) fmt ./...

fmt-check:
	@test -z "$$(gofmt -l .)" || { \
		echo "Go files need formatting:" >&2; \
		gofmt -l . >&2; \
		exit 1; \
	}

$(GOLANGCI_LINT):
	mkdir -p "$(TOOLS_DIR)"
	GOBIN="$(abspath $(TOOLS_DIR))" $(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

lint: $(GOLANGCI_LINT)
	"$(GOLANGCI_LINT)" run ./...

deps:
	$(GO) mod tidy -diff
	$(GO) mod verify

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

build:
	mkdir -p "$(BIN_DIR)"
	$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o "$(BIN_DIR)/abox" ./cmd/abox
	$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o "$(BIN_DIR)/aboxd" ./cmd/aboxd
	$(GO) build ./...
	@test "$$('$(BIN_DIR)/abox' --version)" = "abox $(VERSION) (commit $(COMMIT), built $(BUILD_DATE))"
	@test "$$('$(BIN_DIR)/aboxd' --version)" = "aboxd $(VERSION) (commit $(COMMIT), built $(BUILD_DATE))"

# Stage on the destination filesystem, then rename each binary atomically.
# Never truncate an installed executable that may still be running.
install: build
	@set -eu; \
		mkdir -p "$(INSTALL_DIR)"; \
		stage=$$(mktemp -d "$(INSTALL_DIR)/.abox-install.XXXXXX"); \
		trap 'rm -rf "$$stage"' EXIT HUP INT TERM; \
		for binary in abox aboxd; do \
			test ! -d "$(INSTALL_DIR)/$$binary"; \
			install -m 0755 "$(BIN_DIR)/$$binary" "$$stage/$$binary"; \
		done; \
		for binary in abox aboxd; do \
			mv -f "$$stage/$$binary" "$(INSTALL_DIR)/$$binary"; \
		done

smoke:
	$(GO) run ./cmd/abox-spike list --json

automation-check:
	python3 -m unittest discover -s scripts/tests -p 'test_*.py'

check: fmt-check lint deps test test-race vet build smoke automation-check

clean:
	rm -rf -- "$(BIN_DIR)" "$(CURDIR)/.tools"
