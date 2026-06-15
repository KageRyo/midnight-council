GO ?= ./.conda-go/bin/go
GO_ENV := CGO_ENABLED=0 GOCACHE=/tmp/go-build GOPATH=/tmp/go

.PHONY: fmt test run

fmt:
	$(GO)fmt -w cmd internal

test:
	$(GO_ENV) $(GO) test ./...

run:
	$(GO_ENV) $(GO) run ./cmd/server

