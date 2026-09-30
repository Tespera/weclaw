# weclaw build and install.
#
#   make build    build ./bin/weclaw
#   make test     run tests
#   make install  install to $(PREFIX)/bin and restart a running bridge
#
# Install replaces the binary by rename (new inode), so a running bridge keeps
# its image until restarted; overwriting in place can get it SIGKILLed on macOS.

PREFIX  ?= $(HOME)/.local
BIN     := $(PREFIX)/bin/weclaw
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X weclaw/cmd.Version=$(VERSION) -X weclaw/cmd.SourceDir=$(CURDIR)

.PHONY: build test install dev

build:
	go build -ldflags "$(LDFLAGS)" -o bin/weclaw .

test:
	go test ./...

install: build
	@mkdir -p $(PREFIX)/bin
	@cp bin/weclaw $(BIN).new && mv -f $(BIN).new $(BIN)
	@echo "Installed $(BIN) ($(VERSION))"
	@if $(BIN) status | grep -q "is running"; then $(BIN) restart; fi

dev:
	air -c .air.toml
