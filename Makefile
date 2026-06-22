# Makefile for jl (joblog). Targets wrap the workflow documented in CONTRIBUTING.md.
# Run inside the harv container when a Go toolchain is needed (see AGENTS.md).

BINARY := jl
PKG    := ./cmd/jl
GOBIN  := $(shell go env GOPATH)/bin

.DEFAULT_GOAL := help

## help: list available targets
.PHONY: help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //' | awk -F': ' '{printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

## build: compile the jl binary to ./$(BINARY)
.PHONY: build
build:
	go build -o $(BINARY) $(PKG)

## install: install jl into $(GOBIN)
.PHONY: install
install:
	go install $(PKG)

## run: run jl (pass args with ARGS=...)
.PHONY: run
run:
	go run $(PKG) $(ARGS)

## test: run the full test suite
.PHONY: test
test:
	go test ./...

## update-golden: regenerate golden fixtures (State.Render, etc.)
.PHONY: update-golden
update-golden:
	go test ./internal/state/... -update

## fmt: format all Go source in place
.PHONY: fmt
fmt:
	gofmt -w .

## vet: report formatting and vet problems (CI-style, no writes)
.PHONY: vet
vet:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needs to run on:"; echo "$$unformatted"; exit 1; \
	fi
	go vet ./...

## ci: the pre-PR gate (vet + tests)
.PHONY: ci
ci: vet test

## catalog: refresh the embedded company snapshot (downloads the public CSV)
.PHONY: catalog
catalog:
	go run ./internal/catalog/gen/main.go $(if $(CSV),-in $(CSV),)

## tidy: prune and verify go.mod/go.sum
.PHONY: tidy
tidy:
	go mod tidy

## clean: remove build artifacts
.PHONY: clean
clean:
	rm -f $(BINARY) joblog
	go clean
