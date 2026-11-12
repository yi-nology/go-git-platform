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
#   make check       - fmt + vet + lint + test (CI gate)
#   make clean       - remove generated artifacts

GO ?= go
GOLANGCI_LINT ?= golangci-lint
TEST_FLAGS ?= -race -count=1
COVER_PROFILE ?= coverage.out
# Root-module jobs explicitly disable the workspace so `make check` keeps
# covering only the root module (see AGENTS.md 规则四). Override with
# `make GOWORK=on` to build in workspace mode.
GOWORK ?= off

.PHONY: all build test test-short cover lint fmt vet tidy check check-mcp clean install-tools

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

## check is the CI gate: fmt + vet + lint + test.
check: vet lint test

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
