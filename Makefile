BINARY := limiar
CMD    := ./cmd/limiar

.PHONY: test vet race verify

test:
	go test ./...

vet:
	go vet ./...

race:
	go test -race ./...

verify: vet test race
	bash scripts/ci/check-no-legacy-imports.sh
