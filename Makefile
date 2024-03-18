all: build

.PHONY: build
build:
	@echo "Building indexer binary"
	go build -o build/indexer ./cmd

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
	fieldalignment -fix ./...

.PHONY: test
test:
	go clean -testcache
	go test -v ./...