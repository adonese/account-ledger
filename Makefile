
.PHONY: lint fmt

lint:
	golangci-lint config verify
	golangci-lint run ./... --tests=false

fmt:
	golangci-lint fmt