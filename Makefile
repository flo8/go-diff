# Common tasks. Run "make" or "make check" before committing.
#
# The lint and vulnerability tools run with "go run tool@version": nothing
# to install, and go.mod is not changed.

BINARY := godiff
CMD    := ./cmd/godiff
BIN    := bin

GOVULNCHECK := golang.org/x/vuln/cmd/govulncheck@latest
STATICCHECK := honnef.co/go/tools/cmd/staticcheck@latest

.PHONY: check build install test race vet fmt fmt-check lint vulncheck tidy clean

# Everything that should pass before a commit.
check: fmt-check vet lint test vulncheck

# Build into ./bin/godiff.
build:
	go build -o $(BIN)/$(BINARY) $(CMD)

# Install godiff into $(go env GOPATH)/bin, usually ~/go/bin.
install:
	go install $(CMD)

test:
	go test ./...

# Tests with the race detector (slower).
race:
	go test -race ./...

vet:
	go vet ./...

# Rewrite files with gofmt.
fmt:
	gofmt -w .

# Fail if any file is not gofmt-formatted.
fmt-check:
	@files=$$(gofmt -l .); \
	if [ -n "$$files" ]; then echo "not gofmt-formatted:"; echo "$$files"; exit 1; fi

lint:
	go run $(STATICCHECK) ./...

# Known vulnerabilities in the code and its dependencies.
vulncheck:
	go run $(GOVULNCHECK) ./...

tidy:
	go mod tidy

clean:
	rm -rf $(BIN)
