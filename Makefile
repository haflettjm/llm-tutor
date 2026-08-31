BINARY     := knumble-tutor
ACP_BINARY := knumble-acp
PREFIX     ?= $(HOME)/.local
SERVICE    := knumble-tutor.service
UNIT_DIR   ?= $(HOME)/.config/systemd/user

.PHONY: build test vet fmt check run acp install install-service uninstall-service clean

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

# Run the daemon under systemd so it is up whenever an editor or CLI reads the
# MCP registration it wrote into ~/.claude.json. Without this the daemon has to
# be started by hand and every session started while it is down fails with
# "SSE error: Unable to connect".
install-service: install
	install -d $(UNIT_DIR)
	install -m 0644 packaging/systemd/$(SERVICE) $(UNIT_DIR)/$(SERVICE)
	systemctl --user daemon-reload
	systemctl --user enable --now $(SERVICE)
	@echo
	@echo "$(SERVICE) is enabled and running."
	@echo "Restart your editor or CLI so it reconnects to the MCP server."

uninstall-service:
	-systemctl --user disable --now $(SERVICE)
	rm -f $(UNIT_DIR)/$(SERVICE)
	systemctl --user daemon-reload

clean:
	rm -rf bin/
