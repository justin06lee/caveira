BINARY  := caveira
BINDIR  ?= $(HOME)/.local/bin
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all build install update tui editor test

# The golden path: build the terminal client and the cell editor, install
# `caveira` on PATH. Safe to run again at any time.
all: build install

build: tui editor

install: tui
	mkdir -p $(BINDIR)
	install -m 0755 tui/bin/$(BINARY) $(BINDIR)/$(BINARY)

# caveira has no daemons to stop. Removing the old binary before installing
# gives the new one a fresh inode, so sessions already running keep working.
update:
	rm -f $(BINDIR)/$(BINARY) tui/bin/$(BINARY)
	$(MAKE) all

tui:
	cd tui && go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) .

test:
	cd tui && go vet ./... && go test ./...

# The cell editor: typecheck and bundle. Run it with `bun run dev` in cell-editor.
editor:
	cd cell-editor && bun install && bun run build
