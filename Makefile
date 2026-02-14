BINARY  := gun
PKG     := ./...
GOBIN   ?= $(shell go env GOPATH)/bin
LDFLAGS := -X main.gunModuleRoot=$(CURDIR)

.PHONY: build install clean test

build:
	go build -ldflags '$(LDFLAGS)' -o $(BINARY) .

install:
	go install -ldflags '$(LDFLAGS)' .

clean:
	rm -f $(BINARY)

test:
	go test $(PKG)
