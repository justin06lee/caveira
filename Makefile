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
# actool, which compiles the Dock icon, comes with Xcode. The Xcode in use
# is tried first, then any other Spotlight knows of (one on another
# volume, say). It is run directly rather than through xcrun, which also
# wants the Xcode license agreed with sudo.
XCODE_DEV := $(shell { xcode-select -p 2>/dev/null; 	mdfind "kMDItemCFBundleIdentifier == 'com.apple.dt.Xcode'" 2>/dev/null | sed 's|$$|/Contents/Developer|'; } | 	while IFS= read -r d; do if [ -x "$$d/usr/bin/actool" ]; then echo "$$d"; break; fi; done)
endif

MODULES := core tui desktop

# The steps of an update run in order, even under make -j.
.NOTPARALLEL:

.PHONY: all build install update test deps tui desktop place place-tui place-desktop strays quit-desktop reset-permissions launch

# The golden path, and the update: everything new, everywhere caveira is
# on this machine, then the app opened. Safe to run again at any time.
all: update

# Build both clients, leaving what is installed alone.
build: deps tui desktop

# Build, then put `caveira` and `cav` on PATH and the app in place
# (caveira.app in /Applications on macOS, caveira-desktop and its launcher
# on Linux), without opening it.
install: build
	@$(MAKE) --no-print-directory place

# Everything is built before anything installed is touched, so a build
# that fails leaves the old caveira working. Then the app is quit, the old
# copies are deleted, the new ones go in, and the app is opened again.
# caveira has no daemons; a terminal session already running keeps its old
# binary and carries on.
update: build
	@$(MAKE) --no-print-directory place launch

# Go's module cache is read-only, but a cleaner that sweeps folders by
# name can still empty part of it, and then a build fails on a package the
# cache claims to have. go mod verify finds such a module, and it is
# unpacked again from the download it came from. Go's build cache keeps an
# index of each module's folders, made while it was broken, so that goes
# too.
deps:
	@for m in $(MODULES); do \
		(cd $$m && go mod download) || exit 1; \
		(cd $$m && go mod verify 2>&1) | \
		sed -n 's/^\([^ ]*\) \([^ ]*\): dir has been modified (\(.*\))$$/\1@\2 \3/p' | \
		while read -r mod dir; do \
			echo "repairing $$mod in the Go module cache"; \
			chmod -R u+w "$$dir" && rm -rf "$$dir" && (cd $$m && go mod download $$mod) && go clean -cache || exit 1; \
		done || exit 1; \
	done

place: quit-desktop reset-permissions place-tui place-desktop strays

test:
	cd core && go vet ./... && go test ./...
	cd tui && go vet ./... && go test ./...
	cd desktop && go vet ./... && go test ./...

tui:
	cd tui && go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) .

# `cav` is a link to `caveira`, so either name starts it. The old binary is
# removed rather than written over, so a session running it keeps its file.
place-tui:
	mkdir -p $(BINDIR)
	rm -f $(BINDIR)/$(BINARY) $(BINDIR)/$(ALIAS)
	install -m 0755 tui/bin/$(BINARY) $(BINDIR)/$(BINARY)
	ln -sf $(BINARY) $(BINDIR)/$(ALIAS)

# Copies of caveira elsewhere on PATH, like an old `go install` in
# ~/go/bin, become links to the one just installed, so whichever comes
# first on PATH runs this build. Only Go binaries built from caveira's
# repository are touched.
strays:
	@for f in $$(which -a $(BINARY) $(ALIAS) 2>/dev/null | sort -u); do \
		case "$$f" in $(BINDIR)/*) continue;; esac; \
		[ "$$(readlink "$$f")" = "$(BINDIR)/$(BINARY)" ] && continue; \
		go version -m "$$f" 2>/dev/null | grep -q 'github.com/justin06lee/caveira' || continue; \
		if ln -sf "$(BINDIR)/$(BINARY)" "$$f" 2>/dev/null; then echo "$$f now runs this build"; \
		else echo "$$f is an old caveira that could not be replaced" >&2; fi; \
	done

ifeq ($(UNAME),Darwin)

# Wails builds the frontend with bun, compiles, and packages the .app.
# macOS 26 shades a classic .icns with glass in the Dock, so the app also
# carries an Icon Composer icon with glass, highlight and shadow turned off;
# without an Xcode (see XCODE_DEV) the app keeps the classic icon. actool's
# paths are absolute because it resolves relative ones in a helper process.
desktop:
	cd desktop && CGO_CFLAGS=$(MACOS_MIN) CGO_LDFLAGS=$(MACOS_MIN) \
		$(WAILS) build -clean -skipbindings -ldflags "$(LDFLAGS)"
	@if [ -n "$(XCODE_DEV)" ]; then \
		DEVELOPER_DIR="$(XCODE_DEV)" "$(XCODE_DEV)/usr/bin/actool" $(CURDIR)/desktop/build/darwin/caveira.icon --app-icon caveira \
			--compile $(CURDIR)/desktop/build/bin/$(APP)/Contents/Resources \
			--platform macosx --target-device mac --minimum-deployment-target 13.0 \
			--output-partial-info-plist $(CURDIR)/desktop/build/bin/icon.plist >/dev/null && \
		rm -f desktop/build/bin/$(APP)/Contents/Resources/caveira.icns && \
		codesign --force --sign - desktop/build/bin/$(APP); \
	else echo "no Xcode found: the app keeps the classic icon, which the Dock shades with glass"; fi

place-desktop:
	rm -rf $(APPDIR)/$(APP)
	ditto desktop/build/bin/$(APP) $(APPDIR)/$(APP)

# The app is asked to quit (it stops its chats on the way out) and waited
# for: the new one cannot open while the old one still runs.
quit-desktop:
	-@osascript -e 'if application id "$(BUNDLE_ID)" is running then tell application id "$(BUNDLE_ID)" to quit' 2>/dev/null
	@for i in $$(seq 50); do pgrep -f '$(APPDIR)/$(APP)/Contents/MacOS/' >/dev/null || exit 0; sleep 0.2; done; \
		echo "caveira did not quit; close it and run make update again" >&2; exit 1

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
place-desktop:
	mkdir -p $(BINDIR) $(DATADIR)/applications $(DATADIR)/icons/hicolor/scalable/apps
	rm -f $(BINDIR)/$(DESKTOP_BIN)
	install -m 0755 desktop/build/bin/$(BINARY) $(BINDIR)/$(DESKTOP_BIN)
	install -m 0644 assets/caveira-icon.svg $(DATADIR)/icons/hicolor/scalable/apps/caveira.svg
	sed 's|@BIN@|$(BINDIR)/$(DESKTOP_BIN)|' desktop/build/linux/caveira.desktop > $(DATADIR)/applications/caveira.desktop
	-@if [ -f $(DATADIR)/icons/hicolor/icon-theme.cache ]; then gtk-update-icon-cache -q -t -f $(DATADIR)/icons/hicolor; fi
	-@update-desktop-database -q $(DATADIR)/applications 2>/dev/null

# A process name is cut to 15 characters, which caveira-desktop just fits.
quit-desktop:
	-@pkill -x $(DESKTOP_BIN) 2>/dev/null; true
	@for i in $$(seq 50); do pgrep -x $(DESKTOP_BIN) >/dev/null || exit 0; sleep 0.2; done; \
		echo "caveira did not quit; close it and run make update again" >&2; exit 1

reset-permissions:

# Opened in the background, detached from make; without a display (over
# SSH) it is left for the app grid.
launch:
	@if [ -n "$$DISPLAY$$WAYLAND_DISPLAY" ]; then \
		setsid -f $(BINDIR)/$(DESKTOP_BIN) >/dev/null 2>&1 </dev/null; \
	else echo "no display: open caveira from the app grid"; fi

endif
