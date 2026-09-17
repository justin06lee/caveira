BINARY := caveira
BINDIR ?= $(HOME)/.local/bin

.PHONY: all build install update

# The golden path: build the TUI and put `caveira` on PATH.
all: install

build:
	cd tui && go build -o bin/$(BINARY) .

install: build
	mkdir -p $(BINDIR)
	install -m 0755 tui/bin/$(BINARY) $(BINDIR)/$(BINARY)

# caveira has no daemons to stop. Removing the old binary before installing
# gives the new one a fresh inode, so sessions already running keep working.
update:
	rm -f $(BINDIR)/$(BINARY) tui/bin/$(BINARY)
	$(MAKE) install
