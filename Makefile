BIN_DIR := $(shell pwd)/bin

# Tool versions
MDBOOK_VERSION = 0.4.37
MDBOOK := $(BIN_DIR)/mdbook

# Test tools
STATICCHECK = $(BIN_DIR)/staticcheck

.PHONY: all
all: test

.PHONY: book
book: $(MDBOOK)
	rm -rf docs/book
	cd docs; $(MDBOOK) build


.PHONY: test
test:
	if find . -name go.mod | grep -q go.mod; then \
		$(MAKE) test-go; \
	fi

.PHONY: test-go
test-go: test-tools
	test -z "$$(gofmt -s -l . | tee /dev/stderr)"
	$(STATICCHECK) ./...
	go install ./...
	go test -race -v ./...
	go vet ./...


##@ Tools

$(MDBOOK):
	mkdir -p bin
	curl -fsL https://github.com/rust-lang/mdBook/releases/download/v$(MDBOOK_VERSION)/mdbook-v$(MDBOOK_VERSION)-x86_64-unknown-linux-gnu.tar.gz | tar -C bin -xzf -

.PHONY: test-tools
test-tools: $(STATICCHECK)

$(STATICCHECK):
	mkdir -p $(BIN_DIR)
	GOBIN=$(BIN_DIR) go install honnef.co/go/tools/cmd/staticcheck@latest
