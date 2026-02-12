BINARY_NAME := payments-service
BUILD_DIR := build
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS := -X github.com/vibast-solutions/ms-go-payments/cmd.Version=$(VERSION) \
           -X github.com/vibast-solutions/ms-go-payments/cmd.Commit=$(COMMIT)

.PHONY: build build-linux-arm64 build-linux-amd64 build-darwin-arm64 build-darwin-amd64 build-all clean test e2e proto

build:
	@mkdir -p $(BUILD_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) .

build-linux-arm64:
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 .

build-linux-amd64:
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 .

build-darwin-arm64:
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 .

build-darwin-amd64:
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 .

build-all: build build-linux-arm64 build-linux-amd64 build-darwin-arm64 build-darwin-amd64

test:
	go test ./...

e2e:
	bash e2e/run.sh

proto:
	PATH="$$HOME/go/bin:$$PATH" ./scripts/gen_proto.sh

clean:
	rm -rf $(BUILD_DIR)
