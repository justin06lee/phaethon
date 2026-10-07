# phaethon — computers for every agent on this machine.
#
#   make            build hollow and bangboo, bundle them into phaethon, install it,
#                   and run phaethon install: bangboo registered with every agent harness
#   make build      just produce ./build/phaethon, with everything bundled
#   make install    put phaethon on $PATH
#   make update     rebuild everything, then phaethon sync: this machine, and every host
#   make dist       a release tarball for this platform, in ./dist
#
# hollow and bangboo are built from checkouts beside this one (../hollow,
# ../bangboo), or cloned into .cache when those are not there. A release pins
# them to tags instead: see versions.env.

BINARY  := phaethon
BUILD   := build
DIST    := dist
VERSION := $(shell git describe --tags --match 'v*' --dirty --always 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
GOOS    := $(shell go env GOOS)
GOARCH  := $(shell go env GOARCH)
REPO    := https://github.com/justin06lee

# Tags to build hollow and bangboo from, for a release. With one set, that
# program is cloned at the tag into .cache even when a checkout sits beside
# this one, so a release is never built from someone's working tree; bangboo
# is then built against the hollow its go.mod names, as its own release is.
# Unset, the checkouts beside this one are used as they are.
HOLLOW_REF  ?=
BANGBOO_REF ?=

HOLLOW_SRC  ?= $(if $(HOLLOW_REF),.cache/hollow-$(HOLLOW_REF),$(if $(wildcard ../hollow/go.mod),../hollow,.cache/hollow))
BANGBOO_SRC ?= $(if $(BANGBOO_REF),.cache/bangboo-$(BANGBOO_REF),$(if $(wildcard ../bangboo/go.mod),../bangboo,.cache/bangboo))

# On Linux everything phaethon carries is pure Go. Built without cgo it is
# static, and runs on any distribution, glibc or not.
ifeq ($(GOOS),linux)
export CGO_ENABLED := 0
endif

PREFIX ?= $(shell if [ -w /usr/local/bin ]; then echo /usr/local; else echo $(HOME)/.local; fi)
BINDIR := $(PREFIX)/bin

.PHONY: all build bundle sources install uninstall update check-path test vet fmt check clean \
        dist checksums

all: build install check-path
	@$(BINDIR)/$(BINARY) install

sources:
	@for p in hollow bangboo; do \
		if [ $$p = hollow ]; then d=$(HOLLOW_SRC); r=$(HOLLOW_REF); else d=$(BANGBOO_SRC); r=$(BANGBOO_REF); fi; \
		[ -f $$d/go.mod ] && continue; \
		if [ -n "$$r" ]; then \
			echo "  clone   $(REPO)/$$p at $$r"; \
			git -c advice.detachedHead=false clone -q --depth 1 --branch $$r $(REPO)/$$p $$d || exit 1; \
		else \
			echo "  clone   $(REPO)/$$p"; git clone -q $(REPO)/$$p $$d || exit 1; \
		fi; \
	done

# The binaries phaethon carries, built by their own Makefiles so they are
# built exactly as they would be on their own. They are for the platform
# phaethon is being built for: GOOS and GOARCH reach their Makefiles, and on
# a Mac Go tells clang the architecture for bangboo's cgo. Whatever an earlier
# build left here goes first, since everything here is embedded.
bundle: sources
	@find bundle -type f ! -name README -delete
	@$(MAKE) --no-print-directory -C $(HOLLOW_SRC) build build-linux >/dev/null
	@$(if $(BANGBOO_REF),GOWORK=off) $(MAKE) --no-print-directory -C $(BANGBOO_SRC) build >/dev/null
	@cp $(HOLLOW_SRC)/build/hollow-linux-amd64 bundle/hollow-linux-amd64
	@cp $(HOLLOW_SRC)/build/hollow bundle/hollow-$(GOOS)-$(GOARCH)
	@cp $(BANGBOO_SRC)/build/bangboo bundle/bangboo-$(GOOS)-$(GOARCH)
	@printf '{"phaethon":"%s","hollow":"%s","bangboo":"%s"}\n' "$(VERSION)" \
		"$$(git -C $(HOLLOW_SRC) describe --tags --match 'v*' --dirty --always 2>/dev/null || echo dev)" \
		"$$(git -C $(BANGBOO_SRC) describe --tags --match 'v*' --dirty --always 2>/dev/null || echo dev)" > bundle/versions.json
	@echo "  bundle  $$(cat bundle/versions.json)"

build: bundle
	@mkdir -p $(BUILD)
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(BUILD)/$(BINARY) .

install: build
	@mkdir -p $(BINDIR)
	install -m 0755 $(BUILD)/$(BINARY) $(BINDIR)/$(BINARY)

uninstall:
	@[ -x $(BINDIR)/$(BINARY) ] && $(BINDIR)/$(BINARY) uninstall || true
	@rm -f $(BINDIR)/$(BINARY)

# There is no daemon on this machine — harnesses start bangboo themselves —
# so "restart" here means sync: re-register with every harness, and bring
# every host phaethon manages up to the hollow it now carries.
update: build install check-path
	@$(BINDIR)/$(BINARY) sync

# A release tarball for one platform, dist/phaethon-<os>-<arch>.tar.gz:
# phaethon, with that platform's bangboo and hollow inside it, and the
# license and README beside it. Each platform is its own build, since each
# carries different binaries; this one is for the machine it runs on, or
# for GOARCH when that is given (make dist GOARCH=amd64 on an Apple silicon Mac).
dist: build
	@rm -rf $(DIST)/.stage && mkdir -p $(DIST)/.stage
	@cp $(BUILD)/$(BINARY) LICENSE README.md $(DIST)/.stage/
	@COPYFILE_DISABLE=1 tar --no-xattrs -czf $(DIST)/$(BINARY)-$(GOOS)-$(GOARCH).tar.gz \
		-C $(DIST)/.stage $(BINARY) LICENSE README.md
	@rm -rf $(DIST)/.stage
	@echo "  dist    $(DIST)/$(BINARY)-$(GOOS)-$(GOARCH).tar.gz"
	@$(MAKE) --no-print-directory checksums

# What a release publishes beside the tarballs, in the format both
# `sha256sum -c` and `shasum -a 256 -c` check. A release gathers every
# platform's tarball into one dist/ and runs this again over all of them.
checksums:
	@cd $(DIST) && if command -v sha256sum >/dev/null 2>&1; then sha256sum $(BINARY)-*.tar.gz; \
		else shasum -a 256 $(BINARY)-*.tar.gz; fi > SHA256SUMS
	@cat $(DIST)/SHA256SUMS

check-path:
	@case ":$$PATH:" in *":$(BINDIR):"*) ;; *) \
		echo; echo "  $(BINDIR) is not on your PATH; add it:"; echo "      export PATH=\"$(BINDIR):\$$PATH\"";; esac

check: fmt vet test

test:
	@go test ./...

vet:
	@go vet ./... && echo "  vet ok"

fmt:
	@gofmt -l . | grep -v '^$$' && { echo "unformatted files above"; exit 1; } || echo "  fmt ok"

clean:
	@rm -rf $(BUILD) $(DIST) .cache
	@find bundle -type f ! -name README -delete
