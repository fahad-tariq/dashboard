.PHONY: lint build test run docker-build docker-run clean tidy

BIN := projects
CMD := ./cmd/projects

lint:
	golangci-lint run ./...

build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/$(BIN) $(CMD)

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
