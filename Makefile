BINARY     := knumble-tutor
ACP_BINARY := knumble-acp
PREFIX     ?= $(HOME)/.local

.PHONY: build test vet fmt check run acp install clean

build:
	go build -o bin/$(BINARY) ./cmd/$(BINARY)
	go build -o bin/$(ACP_BINARY) ./cmd/$(ACP_BINARY)

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w $(shell find . -name '*.go' -not -path './vendor/*')

check: fmt vet test

run: build
	./bin/$(BINARY)

# Run the ACP adapter against a running daemon, for manual protocol testing.
acp: build
	./bin/$(ACP_BINARY)

install: build
	install -d $(PREFIX)/bin
	install -m 0755 bin/$(BINARY) $(PREFIX)/bin/$(BINARY)
	install -m 0755 bin/$(ACP_BINARY) $(PREFIX)/bin/$(ACP_BINARY)

clean:
	rm -rf bin/
