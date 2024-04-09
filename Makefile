all: build

.PHONY: build
build:
	@echo "Building indexer binary"
	go build -o build/tx-indexer ./cmd

.PHONY: lint
lint:
	golangci-lint run --config .github/golangci.yaml

.PHONY: gofumpt
gofumpt:
	go install mvdan.cc/gofumpt@latest
	gofumpt -l -w .

.PHONY: fixalign
fixalign:
	go install golang.org/x/tools/go/analysis/passes/fieldalignment/cmd/fieldalignment@latest
	fieldalignment -fix $(filter-out $@,$(MAKECMDGOALS)) # the full package name (not path!)

.PHONY: generate
generate:
	go generate ./...

.PHONY: test
test:
	go clean -testcache
	go test -v ./...