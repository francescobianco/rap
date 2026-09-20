PREFIX  ?= $(HOME)/.local
BINDIR  ?= $(PREFIX)/bin
BIN     ?= rap
DIST    ?= dist

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

.PHONY: build install uninstall test vet fmt check dist clean

build:
	go build -ldflags '$(LDFLAGS)' -o $(BIN) .

install: build
	mkdir -p $(BINDIR)
	install -m 0755 $(BIN) $(BINDIR)/$(BIN)
	@echo "installed $(BINDIR)/$(BIN) ($(VERSION))"

uninstall:
	rm -f $(BINDIR)/$(BIN)

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

check: vet test
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed:"; gofmt -l .; exit 1; }

# Cross-compile every released platform into $(DIST) and write SHA256SUMS.
dist: clean
	@mkdir -p $(DIST)
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; arch=$${platform#*/}; \
		ext=""; [ "$$os" = "windows" ] && ext=".exe"; \
		out="$(DIST)/$(BIN)_$${os}_$${arch}$$ext"; \
		echo "building $$out"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch \
			go build -trimpath -ldflags '$(LDFLAGS)' -o "$$out" . || exit 1; \
	done
	@cd $(DIST) && sha256sum * > SHA256SUMS
	@echo "artifacts in $(DIST)/"

clean:
	rm -f $(BIN)
	rm -rf $(DIST)
