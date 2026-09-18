BINARY  := caveira
BINDIR  ?= $(HOME)/.local/bin
# The backend the installed CLI talks to. Override for a release build:
# `make API_URL=https://your.deployment`.
API_URL ?= http://localhost:3000
LDFLAGS := -s -w -X github.com/justin06lee/caveira/tui/internal/config.defaultBaseURL=$(API_URL)

.PHONY: all build install update web-deps db tui web

# The golden path: web dependencies, database migrations, both builds, and
# `caveira` installed on PATH. Safe to run again at any time.
all: build db install

build: tui web

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

web: web-deps
	cd web-client && bun run build

web-deps:
	cd web-client && bun install

# Applies drizzle/ to the local SQLite file, or to Turso when
# TURSO_DATABASE_URL is set in the environment or web-client/.env.
db: web-deps
	cd web-client && bun run db:migrate
