.PHONY: all build hub client tidy lint test clean

all: build

build: hub client

hub:
	go build -o bin/claude-room-hub ./cmd/hub

client:
	go build -o bin/claude-room ./cmd/claude-room

tidy:
	go mod tidy

lint:
	golangci-lint run ./...

test:
	go test ./...

test-verbose:
	go test -v ./...

clean:
	rm -rf bin/

run-hub:
	go run ./cmd/hub --addr 0.0.0.0:7777

install:
	go install ./cmd/hub
	go install ./cmd/claude-room
