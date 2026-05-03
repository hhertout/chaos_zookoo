BINARY_NAME := chaos_zookoo
BUILD_DIR := bin
CMD_PATH := ./cmd/chaos_zookoo

.PHONY: all build run test lint fmt vet vuln gosec tidy clean check

all: check build

## Build
build:
	go build -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_PATH)

run: build
	$(BUILD_DIR)/$(BINARY_NAME)

clean:
	rm -rf $(BUILD_DIR)

## Quality
fmt:
	gofumpt -w .

lint:
	golangci-lint run ./...

vet:
	go vet ./...

vuln:
	govulncheck ./...

gosec:
	gosec -exclude-generated ./...

## Tests
test:
	go test -v -race ./...

test-cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	@rm -f coverage.out

## Deps
tidy:
	go mod tidy

## All checks (CI-friendly)
check: tidy fmt vet lint vuln gosec test
