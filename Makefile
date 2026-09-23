# phaethon — computers for every agent on this machine.
#
#   make            build hollow and bangboo, bundle them into phaethon, install it,
#                   and run phaethon install: bangboo registered with every agent harness
#   make build      just produce ./build/phaethon, with everything bundled
#   make install    put phaethon on $PATH
#   make update     rebuild everything, then phaethon sync: this machine, and every host
#
# hollow and bangboo are built from checkouts beside this one (../hollow,
# ../bangboo), or cloned into .cache when those are not there.

BINARY  := phaethon
BUILD   := build
VERSION := $(shell git describe --tags --match 'v*' --dirty --always 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
GOOS    := $(shell go env GOOS)
GOARCH  := $(shell go env GOARCH)
REPO    := https://github.com/justin06lee

HOLLOW_SRC  ?= $(if $(wildcard ../hollow/go.mod),../hollow,.cache/hollow)
BANGBOO_SRC ?= $(if $(wildcard ../bangboo/go.mod),../bangboo,.cache/bangboo)

PREFIX ?= $(shell if [ -w /usr/local/bin ]; then echo /usr/local; else echo $(HOME)/.local; fi)
BINDIR := $(PREFIX)/bin

.PHONY: all build bundle sources install uninstall update check-path test vet fmt check clean

all: build install check-path
	@$(BINDIR)/$(BINARY) install

sources:
	@for p in hollow bangboo; do \
		d=$$( [ $$p = hollow ] && echo $(HOLLOW_SRC) || echo $(BANGBOO_SRC) ); \
		if [ ! -f $$d/go.mod ]; then echo "  clone   $(REPO)/$$p"; git clone -q $(REPO)/$$p $$d || exit 1; fi; \
	done

# The binaries phaethon carries, built by their own Makefiles so they are
# built exactly as they would be on their own.
bundle: sources
	@$(MAKE) --no-print-directory -C $(HOLLOW_SRC) build build-linux >/dev/null
	@$(MAKE) --no-print-directory -C $(BANGBOO_SRC) build >/dev/null
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
	@rm -rf $(BUILD) .cache
	@find bundle -type f ! -name README -delete
