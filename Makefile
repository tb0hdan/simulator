.PHONY: test

all: run

run:
	@go run cmd/simulator/main.go

test:
	@go test -race -v ./...
