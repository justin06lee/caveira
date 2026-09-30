BINARY    := caveira
ALIAS     := cav
BINDIR    ?= $(HOME)/.local/bin
APPDIR    ?= /Applications
APP       := caveira.app
BUNDLE_ID := dev.caveira.desktop
VERSION   := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS   := -s -w -X main.version=$(VERSION)
WAILS     ?= go run github.com/wailsapp/wails/v2/cmd/wails@v2.16.0
UNAME     := $(shell uname -s)
# Go builds for macOS 13 and up; Wails would otherwise link for 10.13.
MACOS_MIN := -mmacosx-version-min=13.0
# On Linux the app is a binary beside the terminal client, with a launcher
# and an icon under the XDG data folder so it shows in the app grid.
DESKTOP_BIN := caveira-desktop
DATADIR     ?= $(or $(XDG_DATA_HOME),$(HOME)/.local/share)
# Wails links WebKitGTK 4.0 unless told otherwise; Ubuntu 24.04 and later
# only have 4.1.
WAILS_TAGS  := $(shell pkg-config --exists webkit2gtk-4.1 2>/dev/null && echo -tags webkit2_41)

ifeq ($(UNAME),Darwin)
INSTALLED := $(APPDIR)/$(APP)
else
INSTALLED := $(BINDIR)/$(DESKTOP_BIN)
endif

.PHONY: all build install update test tui desktop install-tui install-desktop quit-desktop reset-permissions launch

# The golden path: build the terminal client and the desktop app, install
# `caveira` and `cav` on PATH and the app (caveira.app in /Applications on
# macOS, caveira-desktop and its launcher on Linux), and open the app.
# Safe to run again at any time.
all: build install launch

build: tui desktop

install: install-tui install-desktop

# caveira has no daemons. The terminal binary is removed before it is
# replaced, so sessions already running keep their inode and carry on; the
# app is quit, deleted, rebuilt, and opened again.
update: quit-desktop
	rm -f $(BINDIR)/$(BINARY) $(BINDIR)/$(ALIAS) tui/bin/$(BINARY)
	rm -rf $(INSTALLED) desktop/build/bin
	$(MAKE) all

test:
	cd core && go vet ./... && go test ./...
	cd tui && go vet ./... && go test ./...
	cd desktop && go vet ./... && go test ./...

tui:
	cd tui && go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) .

# `cav` is a link to `caveira`, so either name starts it.
install-tui: tui
	mkdir -p $(BINDIR)
	install -m 0755 tui/bin/$(BINARY) $(BINDIR)/$(BINARY)
	ln -sf $(BINARY) $(BINDIR)/$(ALIAS)

ifeq ($(UNAME),Darwin)

# Wails builds the frontend with bun, compiles, and packages the .app.
# macOS 26 shades a classic .icns with glass in the Dock, so the app also
# carries an Icon Composer icon with glass, highlight and shadow turned off;
# actool comes with Xcode, and without it the app keeps the classic icon. Its
# paths are absolute because it resolves relative ones in a helper process.
desktop:
	cd desktop && CGO_CFLAGS=$(MACOS_MIN) CGO_LDFLAGS=$(MACOS_MIN) \
		$(WAILS) build -clean -skipbindings -ldflags "$(LDFLAGS)"
	@if xcrun --find actool >/dev/null 2>&1; then \
		xcrun actool $(CURDIR)/desktop/build/darwin/caveira.icon --app-icon caveira \
			--compile $(CURDIR)/desktop/build/bin/$(APP)/Contents/Resources \
			--platform macosx --target-device mac --minimum-deployment-target 13.0 \
			--output-partial-info-plist $(CURDIR)/desktop/build/bin/icon.plist >/dev/null && \
		rm -f desktop/build/bin/$(APP)/Contents/Resources/caveira.icns && \
		codesign --force --sign - desktop/build/bin/$(APP); \
	else echo "actool not found (install Xcode): keeping the classic icon"; fi

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

else

# Wails builds the frontend with bun and compiles a single binary against
# GTK 3 and WebKitGTK.
desktop:
	cd desktop && $(WAILS) build -clean -skipbindings $(WAILS_TAGS) -ldflags "$(LDFLAGS)"

# The launcher names the binary by its full path, since a session started
# from the login screen may not have ~/.local/bin on PATH. The icon is the
# flat one from assets/, as an SVG, which GNOME draws at any size. An icon
# cache is only refreshed where one is already kept: a new one would hide
# the icons other apps put there later without refreshing it.
install-desktop: desktop quit-desktop
	mkdir -p $(BINDIR) $(DATADIR)/applications $(DATADIR)/icons/hicolor/scalable/apps
	install -m 0755 desktop/build/bin/$(BINARY) $(BINDIR)/$(DESKTOP_BIN)
	install -m 0644 assets/caveira-icon.svg $(DATADIR)/icons/hicolor/scalable/apps/caveira.svg
	sed 's|@BIN@|$(BINDIR)/$(DESKTOP_BIN)|' desktop/build/linux/caveira.desktop > $(DATADIR)/applications/caveira.desktop
	-@if [ -f $(DATADIR)/icons/hicolor/icon-theme.cache ]; then gtk-update-icon-cache -q -t -f $(DATADIR)/icons/hicolor; fi
	-@update-desktop-database -q $(DATADIR)/applications 2>/dev/null

# A process name is cut to 15 characters, which caveira-desktop just fits.
quit-desktop:
	-@pkill -x $(DESKTOP_BIN) 2>/dev/null; true

reset-permissions:

# Opened in the background, detached from make; without a display (over
# SSH) it is left for the app grid.
launch:
	@if [ -n "$$DISPLAY$$WAYLAND_DISPLAY" ]; then \
		setsid -f $(BINDIR)/$(DESKTOP_BIN) >/dev/null 2>&1 </dev/null; \
	else echo "no display: open caveira from the app grid"; fi

endif
