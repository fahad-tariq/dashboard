.PHONY: lint fmt build test vuln cover bench e2e run docker-build docker-run clean tidy

BIN := dashboard
CMD := ./cmd/dashboard
GOBIN := $(shell go env GOBIN)
ifeq ($(GOBIN),)
GOBIN := $(shell go env GOPATH)/bin
endif

lint:
	golangci-lint run ./...

fmt:
	golangci-lint fmt ./...

VERSION ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)

build:
	CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=$(VERSION)" -o bin/$(BIN) $(CMD)

test:
	go test -count=1 -race ./...

vuln:
	$(GOBIN)/govulncheck ./...

cover:
	go test -count=1 -coverpkg=./internal/... -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

bench:
	bash -o pipefail -c "go test -run='^$$' -bench=BenchmarkMutate200 -benchmem -count=10 ./test/ | tee bench.txt"
	$(GOBIN)/benchstat bench.txt

e2e:
	bash e2e/run.sh

run:
	go run $(CMD)

tidy:
	go mod tidy

docker-build:
	docker build -t $(BIN) .

docker-run:
	docker compose up

clean:
	rm -rf bin/ coverage.out bench.txt
