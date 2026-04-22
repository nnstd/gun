BINARY  := gun
PKG     := ./...
GOBIN   ?= $(shell go env GOPATH)/bin
GOCACHE ?= /tmp/gun-gocache
LDFLAGS := -X main.gunModuleRoot=$(CURDIR)

.PHONY: build install clean test check

build:
	go build -ldflags '$(LDFLAGS)' -o $(BINARY) .

install:
	go install -ldflags '$(LDFLAGS)' .

clean:
	rm -f $(BINARY)

test:
	mkdir -p $(GOCACHE) && GOCACHE=$(GOCACHE) go test $(PKG)

check:
	mkdir -p $(GOCACHE) && GOCACHE=$(GOCACHE) $(GOBIN)/staticcheck $(PKG)
