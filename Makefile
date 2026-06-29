BINARY := limiar
CMD    := ./cmd/limiar

.PHONY: build run test vet clean install

build:
	go build -o $(BINARY) $(CMD)

run:
	go run $(CMD)

test:
	go test ./...

vet:
	go vet ./...

race:
	go test -race ./...

clean:
	rm -f $(BINARY)

install:
	go install $(CMD)
