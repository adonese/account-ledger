
.PHONY: lint fmt

lint:
	golangci-lint config verify
	golangci-lint run ./...

fmt:
	golangci-lint fmt