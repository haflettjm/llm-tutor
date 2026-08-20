BINARY := knumble-tutor
ACP_BINARY := knumble-acp
SOCKET := /tmp/llm-tutor.sock

.PHONY: build test run clean

build:
	go build -o bin/$(BINARY) ./cmd/$(BINARY)
	go build -o bin/$(ACP_BINARY) ./cmd/$(ACP_BINARY)

test:
	go test ./...

run: build
	ANTHROPIC_API_KEY=$(ANTHROPIC_API_KEY) ./bin/$(BINARY)

clean:
	rm -rf bin/
