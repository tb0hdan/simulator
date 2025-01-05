.PHONY: test
GOLINT_VERSION=v1.62.2

all: run

lint-dep:
	@if [ ! -f $(shell go env GOPATH)/bin/golangci-lint ]; then curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s $(GOLINT_VERSION); fi

lint: lint-dep
	@golangci-lint run ./...

run:
	@go run cmd/simulator/main.go

test:
	@go test -v ./...
