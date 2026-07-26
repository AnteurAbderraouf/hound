.PHONY: help build build-all run test clean docker docker-run tidy demo

BINARY_NAME=hound
BUILD_DIR=bin
CMD_DIR=./cmd/hound
QUERY_CMD_DIR=./cmd/hound-query

help: ## show available commands
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-14s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## build the hound binary into ./bin
	go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_DIR)

build-all: ## build hound and hound-query into ./bin
	go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_DIR)
	go build -ldflags="-s -w" -o $(BUILD_DIR)/hound-query $(QUERY_CMD_DIR)

run: ## run hound locally with `go run`
	go run $(CMD_DIR)

test: ## run all go tests
	go test -v ./...

tidy: ## clean up go.mod
	go mod tidy

clean: ## remove build artifacts
	rm -rf $(BUILD_DIR)

demo: ## fire a batch of DNS queries at a running hound (default: 127.0.0.1:5300)
	go run $(QUERY_CMD_DIR) --server 127.0.0.1:5300 --rounds 1 --sleep 80

docker: ## build the docker image
	docker build -t hound:latest -f docker/Dockerfile .

docker-run: docker ## build and run in docker (host network for DNS)
	docker run --rm --network host -v $(PWD)/data:/data hound:latest
