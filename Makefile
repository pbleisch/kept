SHELL := /bin/sh

PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin
BIN ?= kept
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || printf 'dev')
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || printf 'unknown')
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT)

.PHONY: build test vet install uninstall clean

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/kept

test:
	go test ./...

vet:
	go vet ./...

install: build
	install -m 0755 $(BIN) "$(BINDIR)/kept"

uninstall:
	rm -f "$(BINDIR)/kept"

clean:
	rm -f $(BIN)
