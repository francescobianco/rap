PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin
BIN ?= rap

.PHONY: build install test clean

build:
	go build -o $(BIN) .

install: build
	mkdir -p $(BINDIR)
	cp $(BIN) $(BINDIR)/$(BIN)

test:
	go test ./...

clean:
	rm -f $(BIN)
