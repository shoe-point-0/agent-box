.PHONY: fmt fmt-check test test-race vet build smoke automation-check check

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

automation-check:
	bash -n scripts/alph-snowball-agent scripts/crush-sprout automation/sprout-crusher/bin/git
	real_git="$${HERMES_CRUSHER_REAL_GIT:-$$(command -v git)}"; HERMES_CRUSHER_REAL_GIT="$$real_git" automation/sprout-crusher/bin/git --version >/dev/null
	@if real_git="$${HERMES_CRUSHER_REAL_GIT:-$$(command -v git)}"; HERMES_CRUSHER_REAL_GIT="$$real_git" automation/sprout-crusher/bin/git push >/dev/null 2>&1; then \
		echo "Git push guard failed" >&2; \
		exit 1; \
	fi
	python3 -m unittest discover -s scripts/tests -p 'test_*.py'

check: fmt-check test test-race vet build smoke automation-check
