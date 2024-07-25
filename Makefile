BIN_DIR := $(shell pwd)/bin

# Tool versions

# Test tools
STATICCHECK = $(BIN_DIR)/staticcheck

.PHONY: all
all: test


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

.PHONY: test-tools
test-tools: $(STATICCHECK)

$(STATICCHECK):
	mkdir -p $(BIN_DIR)
	GOBIN=$(BIN_DIR) go install honnef.co/go/tools/cmd/staticcheck@latest
