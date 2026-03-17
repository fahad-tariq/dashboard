.PHONY: lint build test run docker-build docker-run clean tidy

BIN := dashboard
CMD := ./cmd/dashboard

lint:
	golangci-lint run ./...

VERSION ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)

build:
	CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=$(VERSION)" -o bin/$(BIN) $(CMD)

test:
	go test -count=1 -race ./...

run:
	go run $(CMD)

tidy:
	go mod tidy

docker-build:
	docker build -t $(BIN) .

docker-run:
	docker compose up

clean:
	rm -rf bin/
