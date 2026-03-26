VERSION ?= dev
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -s -w \
	-X main.ver=$(VERSION) \
	-X main.gitCommit=$(GIT_COMMIT) \
	-X main.buildDate=$(BUILD_DATE)

.PHONY: build install test lint clean

build:
	go build -ldflags '$(LDFLAGS)' -o clime .

install:
	go install -ldflags '$(LDFLAGS)' .
	@BIN_DIR="$$(go env GOBIN)"; \
	if [ -z "$$BIN_DIR" ]; then BIN_DIR="$$(go env GOPATH)/bin"; fi; \
	if [ -x "$$BIN_DIR/clime" ]; then \
		echo "Running clime init..."; \
		PATH="$$BIN_DIR:$$PATH" "$$BIN_DIR/clime" init || true; \
	fi

test:
	go test ./... -v

lint:
	go vet ./...

clean:
	rm -f clime
