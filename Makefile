# go-git-platform Makefile
#
# Common targets:
#   make test        - run all tests with race detector and coverage
#   make cover       - print coverage summary
#   make lint        - run golangci-lint
#   make fmt         - format all Go files
#   make vet         - run go vet
#   make build       - build all packages
#   make tidy        - run go mod tidy
#   make check       - vet + lint + test + cover-domains (CI gate)
#   make clean       - remove generated artifacts

GO ?= go
GOLANGCI_LINT ?= golangci-lint
TEST_FLAGS ?= -race -count=1
COVER_PROFILE ?= coverage.out
# Root-module jobs explicitly disable the workspace so `make check` keeps
# covering only the root module (see AGENTS.md 规则四). Override with
# `make GOWORK=on` to build in workspace mode.
GOWORK ?= off

# Sandbox write-E2E images (T4). Pins are live-verified against the go.mod
# SDK compile faces (gitea.dev/sdk v1.5.1 gates up to 29.0.0; forgejo-sdk
# v3.0.0 gate 1.22.0 ↔ forgejo "17.0.1+gitea-1.22.0") — see e2e/harness_test.go
# for the evidence trail. Override with GITEA_IMAGE=... make e2e to try another.
GITEA_IMAGE   ?= gitea/gitea:28.4
FORGEJO_IMAGE ?= codeberg.org/forgejo/forgejo:17.0.1

.PHONY: all build test test-short cover cover-domains lint fmt vet tidy check check-mcp e2e clean install-tools

all: build

## build compiles every package in the module.
build:
	GOWORK=$(GOWORK) $(GO) build ./...

## test runs the full test suite with race detection and coverage.
test:
	GOWORK=$(GOWORK) $(GO) test $(TEST_FLAGS) -coverprofile=$(COVER_PROFILE) ./...

## test-short runs only tests marked as short (smoke tests).
test-short:
	GOWORK=$(GOWORK) $(GO) test -short ./...

## cover prints the per-function coverage summary.
cover: test
	$(GO) tool cover -func=$(COVER_PROFILE) | tail -1

## cover-html opens the HTML coverage report in a browser.
cover-html: test
	$(GO) tool cover -html=$(COVER_PROFILE)

## cover-domains asserts the four gated domains (webhook/transport/gitbackend/
## pageiter) each reach >=80% via scripts/cover-domain.sh; any miss exits non-zero.
cover-domains: test
	bash scripts/cover-domain.sh $(COVER_PROFILE)

## lint runs golangci-lint with the project's configuration.
lint:
	GOWORK=$(GOWORK) $(GOLANGCI_LINT) run ./...

## fmt formats every Go file with gofmt and goimports.
fmt:
	GOWORK=$(GOWORK) $(GO) fmt ./...
	@command -v goimports >/dev/null 2>&1 && goimports -w . || true

## vet runs the standard go vet checks.
vet:
	GOWORK=$(GOWORK) $(GO) vet ./...

## tidy syncs go.mod and go.sum with the current source tree.
tidy:
	GOWORK=$(GOWORK) $(GO) mod tidy

## check is the CI gate: vet + lint + test + four-domain coverage floor.
## It does NOT compile the e2e package (//go:build e2e is invisible without
## the tag) — sandbox write roundtrips run via `make e2e` or the e2e workflow.
check: vet lint test cover-domains

## e2e runs the gitea/forgejo sandbox write roundtrips (3期 T4): the harness
## boots one-shot containers locally (docker required — skips with a reason
## when absent); CI injects E2E_GITEA_URL/E2E_FORGEJO_URL + tokens instead.
## Image overrides: GITEA_IMAGE=... FORGEJO_IMAGE=... make e2e
e2e:
	GOWORK=$(GOWORK) GITEA_IMAGE=$(GITEA_IMAGE) FORGEJO_IMAGE=$(FORGEJO_IMAGE) $(GO) test -tags=e2e -count=1 -timeout 15m ./e2e/...

## check-mcp runs the CI gate inside the mcp module (workspace mode, so it
## tests the local SDK, matching the pre-replace development flow).
check-mcp:
	cd mcp && $(GO) vet ./...
	cd mcp && $(GO) test $(TEST_FLAGS) ./...
	cd mcp && $(GOLANGCI_LINT) run ./...

## clean removes generated artifacts.
clean:
	rm -f $(COVER_PROFILE)

## install-tools installs the development tools listed in tools.go.
install-tools:
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	$(GO) install golang.org/x/tools/cmd/goimports@latest
