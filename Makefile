.PHONY: build test clean deps

BINARY_NAME=runtime

build:
	go build ./...

deps:
	go mod download
	go mod tidy

test:
	go test -v ./...

test-unit:
	go test -v ./internal/... -short

test-integration-docker:
	KRANE_RUNTIME_BACKEND=docker go test -v ./tests/integration/... -tags integration

test-integration-kubernetes:
	KRANE_RUNTIME_BACKEND=kubernetes \
	KUBECONFIG=${KUBECONFIG} \
	go test -v ./tests/integration/... -tags integration

clean:
	rm -f $(BINARY_NAME)

install:
	go install ./...

# Run with specific backend
run-docker:
	KRANE_RUNTIME_BACKEND=docker go run ./cmd/...

run-kubernetes:
	KRANE_RUNTIME_BACKEND=kubernetes go run ./cmd/...
