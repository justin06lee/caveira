BINARY    := caveira
ALIAS     := cav
BINDIR    ?= $(HOME)/.local/bin
APPDIR    ?= /Applications
APP       := caveira.app
BUNDLE_ID := dev.caveira.desktop
VERSION   := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS   := -s -w -X main.version=$(VERSION)
WAILS     ?= go run github.com/wailsapp/wails/v2/cmd/wails@v2.16.0
# Go builds for macOS 13 and up; Wails would otherwise link for 10.13.
MACOS_MIN := -mmacosx-version-min=13.0

.PHONY: all build install update test tui desktop install-tui install-desktop quit-desktop reset-permissions launch

# The golden path: build the terminal client and the desktop app, install
# `caveira` and `cav` on PATH and caveira.app in /Applications, and open
# the app. Safe to run again at any time.
all: build install launch

build: tui desktop

install: install-tui install-desktop

# caveira has no daemons. The terminal binary is removed before it is
# replaced, so sessions already running keep their inode and carry on; the
# app is quit, deleted, rebuilt, and opened again.
update: quit-desktop
	rm -f $(BINDIR)/$(BINARY) $(BINDIR)/$(ALIAS) tui/bin/$(BINARY)
	rm -rf $(APPDIR)/$(APP) desktop/build/bin
	$(MAKE) all

test:
	cd core && go vet ./... && go test ./...
	cd tui && go vet ./... && go test ./...
	cd desktop && go vet ./... && go test ./...

tui:
	cd tui && go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) .

# Wails builds the frontend with bun, compiles, and packages the .app.
desktop:
	cd desktop && CGO_CFLAGS=$(MACOS_MIN) CGO_LDFLAGS=$(MACOS_MIN) \
		$(WAILS) build -clean -skipbindings -ldflags "$(LDFLAGS)"

# `cav` is a link to `caveira`, so either name starts it.
install-tui: tui
	mkdir -p $(BINDIR)
	install -m 0755 tui/bin/$(BINARY) $(BINDIR)/$(BINARY)
	ln -sf $(BINARY) $(BINDIR)/$(ALIAS)

install-desktop: desktop quit-desktop reset-permissions
	rm -rf $(APPDIR)/$(APP)
	ditto desktop/build/bin/$(APP) $(APPDIR)/$(APP)

quit-desktop:
	-@osascript -e 'if application id "$(BUNDLE_ID)" is running then tell application id "$(BUNDLE_ID)" to quit' 2>/dev/null

# macOS ties folder access (Documents, Desktop, Downloads, external and
# network volumes) to the app's signature, and every local build is signed
# anew, so a new build would be refused under the old grant while System
# Settings still shows it on. Clearing this app's entries makes the new
# build ask again. System Settings is closed first because it caches them.
reset-permissions:
	-@osascript -e 'quit app "System Settings"' 2>/dev/null
	-@for s in SystemPolicyDocumentsFolder SystemPolicyDesktopFolder SystemPolicyDownloadsFolder \
		SystemPolicyRemovableVolumes SystemPolicyNetworkVolumes; do \
		tccutil reset $$s $(BUNDLE_ID) >/dev/null 2>&1; done

launch:
	open $(APPDIR)/$(APP)
