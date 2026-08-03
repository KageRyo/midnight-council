GO ?= go

.PHONY: build fmt fmt-check vet test test-race js-check run

fmt:
	$(GO) fmt ./...

fmt-check:
	@test -z "$$($(GO)fmt -l cmd internal)" || (echo "Go files are not formatted; run 'make fmt'." && exit 1)

vet:
	$(GO) vet ./...

test:
	CGO_ENABLED=0 $(GO) test ./...

test-race:
	$(GO) test -race ./...

js-check:
	node --check internal/webui/static/app.js

build:
	CGO_ENABLED=0 $(GO) build ./cmd/server

run:
	$(GO) run ./cmd/server
