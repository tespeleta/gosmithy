# gosmithy Makefile
#
# Common targets:
#   make build      build bin/gosmithy (default)
#   make check      gofmt check + go vet + tests
#   make fuzz       run the parser fuzzer for FUZZTIME seconds

GO        ?= go
# The sandbox's default ~/.cache/go-build is read-only; keep the Go build cache
# inside the repository (gitignored) so build/test/vet can write it.
GOCACHE   ?= $(CURDIR)/.gocache
export GOCACHE
BIN_DIR   ?= bin
BIN       ?= $(BIN_DIR)/gosmithy
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo 0.1.0-dev)
FUZZTIME  ?= 10s
LDFLAGS   ?= -s -w -X main.version=$(VERSION)

GOFILES := $(shell find . -name '*.go' -not -path './$(BIN_DIR)/*')

.PHONY: all build test test-fast test-integration test-race vet fmt fmt-check check fuzz run install clean help

all: build

build:
	@mkdir -p $(BIN_DIR)
	$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN) ./cmd/gosmithy

test:
	GOSMITHY_INTEGRATION=1 $(GO) test ./...

test-fast:
	$(GO) test ./...

test-integration:
	GOSMITHY_INTEGRATION=1 $(GO) test ./...

test-race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -w $(GOFILES)

fmt-check:
	@files="$$(gofmt -l $(GOFILES))"; \
	if [ -n "$$files" ]; then \
		echo "gofmt required for:"; \
		echo "$$files"; \
		exit 1; \
	fi

check: fmt-check vet test

fuzz:
	$(GO) test ./idl -run '^$$' -fuzz=FuzzParse -fuzztime=$(FUZZTIME)

run: build
	$(BIN) $(ARGS)

install:
	$(GO) install -ldflags '$(LDFLAGS)' ./cmd/gosmithy

clean:
	rm -rf $(BIN_DIR)

help:
	@printf '%s\n' \
		'Usage: make [target]' \
		'' \
		'Targets:' \
		'  build      Build $(BIN) (default)' \
		'  test       Run all tests, including generated-module compilation'
		'  test-fast  Run tests without generated-module compilation'
		'  test-integration  Run tests with GOSMITHY_INTEGRATION=1' \
		'  test-race  Run all tests with the race detector' \
		'  vet        Run go vet' \
		'  fmt        Format Go sources' \
		'  fmt-check  Fail if sources are not gofmt-clean' \
		'  check      fmt-check + vet + test' \
		'  fuzz       Run the parser fuzzer (FUZZTIME=$(FUZZTIME))' \
		'  run        Build and run the binary (ARGS=...)' \
		'  install    Install the binary into GOBIN/GOPATH/bin' \
		'  clean      Remove build artifacts'
