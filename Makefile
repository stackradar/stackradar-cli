GO_PACKAGES := ./cmd/... ./internal/...
TOOLS_MODFILE := tools/go.mod
GOLANGCI_LINT ?= go tool -modfile=$(TOOLS_MODFILE) github.com/golangci/golangci-lint/v2/cmd/golangci-lint
GORELEASER ?= go tool -modfile=$(TOOLS_MODFILE) github.com/goreleaser/goreleaser/v2
GOVULNCHECK ?= go tool -modfile=$(TOOLS_MODFILE) golang.org/x/vuln/cmd/govulncheck

.PHONY: build fmt fmt-check govulncheck lint release-check snapshot test test-race vet

build:
	go build -o bin/stackradar ./cmd/stackradar

fmt:
	gofmt -w .

fmt-check:
	test -z "$$(gofmt -l .)"

lint:
	$(GOLANGCI_LINT) run $(GO_PACKAGES)

govulncheck:
	$(GOVULNCHECK) $(GO_PACKAGES)

release-check:
	$(GORELEASER) check

snapshot:
	$(GORELEASER) release --snapshot --clean --skip=publish,sign

test:
	go test $(GO_PACKAGES)

test-race:
	go test -race -shuffle=on $(GO_PACKAGES)

vet:
	go vet $(GO_PACKAGES)
