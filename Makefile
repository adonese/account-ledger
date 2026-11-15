
.PHONY: lint fmt bench

lint:
	golangci-lint config verify
	golangci-lint run ./... --tests=false

fmt:
	golangci-lint fmt

# Reproduces the scaling table in DESIGN.md, section 1.
bench:
	cmd/bench/run.sh
