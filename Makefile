VERSION := $(shell node -p "require('./ui/package.json').version")
DIST := dist
BIN_DIR := $(HOME)/.local/bin
D_BIN_DIR := $(HOME)/.lemongrass/bin
LDFLAGS := -ldflags "-X github.com/faizalv/lemongrass/cmd/lgrass/version.Version=$(VERSION)"
BINARIES := lgrass lgrassd lgrassconf

.PHONY: build-linux build-rpm build-mac dist dist-linux dist-darwin dev install verify clean submodules bibliothek-update

dist: dist-linux dist-darwin

# bibliothek is a submodule embedded by internal/bibliothek, so no Go build works without it checked out.
submodules:
	git submodule update --init

# Moves the bibliothek pin to its latest commit; the new pin is committed like any other change.
bibliothek-update: submodules
	git submodule update --remote internal/bibliothek/repo

# Every binary for each target arch, written to dist/<os>-<arch>/ where ui/electron-builder.yml reads them. The old output of that os is removed first so a stale binary is never bundled.
dist-linux: submodules
	rm -rf $(DIST)/linux-amd64 $(DIST)/linux-arm64
	mkdir -p $(DIST)/linux-amd64 $(DIST)/linux-arm64
	for bin in $(BINARIES); do \
		GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(DIST)/linux-amd64/$$bin ./cmd/$$bin || exit 1; \
		GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(DIST)/linux-arm64/$$bin ./cmd/$$bin || exit 1; \
	done

dist-darwin: submodules
	rm -rf $(DIST)/darwin-amd64 $(DIST)/darwin-arm64
	mkdir -p $(DIST)/darwin-amd64 $(DIST)/darwin-arm64
	for bin in $(BINARIES); do \
		GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(DIST)/darwin-amd64/$$bin ./cmd/$$bin || exit 1; \
		GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(DIST)/darwin-arm64/$$bin ./cmd/$$bin || exit 1; \
	done

build-linux: dist-linux
	npm --prefix ui run build:linux

build-rpm: dist-linux
	npm --prefix ui run build:rpm

build-mac: dist-darwin
	npm --prefix ui run build:mac

# Host-arch build for local dev iteration. lgrass goes to ~/.local/bin, lgrassd to ~/.lemongrass/bin, and lgrassconf beside lgrass with its service unit registered. A running executable cannot be overwritten in place, so lgrassconf and lgrassd are built beside the old file and renamed over it.
install: submodules
	mkdir -p $(BIN_DIR) $(D_BIN_DIR)
	go build $(LDFLAGS) -o $(BIN_DIR)/lgrassconf.new ./cmd/lgrassconf
	mv -f $(BIN_DIR)/lgrassconf.new $(BIN_DIR)/lgrassconf
	$(BIN_DIR)/lgrassconf install
	go build $(LDFLAGS) -o $(BIN_DIR)/lgrass ./cmd/lgrass
	go build $(LDFLAGS) -o $(D_BIN_DIR)/lgrassd.new ./cmd/lgrassd
	mv -f $(D_BIN_DIR)/lgrassd.new $(D_BIN_DIR)/lgrassd

# bibliothek-update comes first so install's own submodules prerequisite, already run once here, does not check the old pin back out.
dev: bibliothek-update install
	npm --prefix ui run dev

# Compiles everything and discards the output, since a plain `go build` of a single main package writes its binary into the source directory.
verify: submodules
	go build -a -o /dev/null ./...

clean:
	rm -rf $(DIST) ui/out ui/dist
	rm -f $(BINARIES) $(addsuffix .exe,$(BINARIES))
