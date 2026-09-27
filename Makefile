BINARY  := caveira
ALIAS   := cav
BINDIR  ?= $(HOME)/.local/bin
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all build install update tui editor test

# The golden path: build the terminal client and the cell editor, install
# `caveira` and `cav` on PATH. Safe to run again at any time.
all: build install

build: tui editor

# `cav` is a link to `caveira`, so either name starts it.
install: tui
	mkdir -p $(BINDIR)
	install -m 0755 tui/bin/$(BINARY) $(BINDIR)/$(BINARY)
	ln -sf $(BINARY) $(BINDIR)/$(ALIAS)

# caveira has no daemons to stop. Removing the old binary before installing
# gives the new one a fresh inode, so sessions already running keep working.
update:
	rm -f $(BINDIR)/$(BINARY) $(BINDIR)/$(ALIAS) tui/bin/$(BINARY)
	$(MAKE) all

tui:
	cd tui && go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) .

test:
	cd core && go vet ./... && go test ./...
	cd tui && go vet ./... && go test ./...

# The cell editor: typecheck and bundle. Run it with `bun run dev` in cell-editor.
editor:
	@if [ -d cell-editor ]; then cd cell-editor && bun install && bun run build; \
	else echo "cell-editor/ is not in the tree; skipping it"; fi
