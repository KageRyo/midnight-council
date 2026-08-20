GO ?= go
GOVULNCHECK_VERSION ?= v1.1.4

.PHONY: build fmt fmt-check mod-verify vuln vet test test-race js-check run

fmt:
	$(GO) fmt ./...

fmt-check:
	@test -z "$$($(GO)fmt -l cmd internal)" || (echo "Go files are not formatted; run 'make fmt'." && exit 1)

mod-verify:
	$(GO) mod verify

vuln:
	$(GO) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

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
