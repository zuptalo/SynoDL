.PHONY: run test vet fmt tidy mock

run:        ## Start the proxy (SYNO_URL defaults to the mock DSM in dev)
	go run ./cmd/synodl

mock:       ## Start the fake DSM on :8091
	go run ./cmd/synomock

test:       ## Run all tests (no NAS or network required)
	go test ./...

vet:        ## Static checks
	go vet ./...

fmt:        ## Format the codebase
	gofmt -w .

tidy:       ## Resolve and prune go.mod/go.sum
	go mod tidy
