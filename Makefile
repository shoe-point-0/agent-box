.PHONY: fmt fmt-check test test-race vet build smoke check

GO ?= go

fmt:
	$(GO) fmt ./...

fmt-check:
	@test -z "$$(gofmt -l .)" || { \
		echo "Go files need formatting:" >&2; \
		gofmt -l . >&2; \
		exit 1; \
	}

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

build:
	$(GO) build ./...

smoke:
	$(GO) run ./cmd/abox-spike list --json

check: fmt-check test test-race vet build smoke
