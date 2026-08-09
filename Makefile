BINARY := knumble-tutor
SOCKET := /tmp/knumble-tutor.sock

.PHONY: build test run clean

build:
	go build -o bin/$(BINARY) ./cmd/$(BINARY)

test:
	go test ./...

run: build
	ANTHROPIC_API_KEY=$(ANTHROPIC_API_KEY) ./bin/$(BINARY)

clean:
	rm -rf bin/
