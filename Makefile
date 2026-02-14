BINARY  := gun
PKG     := ./...
GOBIN   ?= $(shell go env GOPATH)/bin

.PHONY: build install clean test

build:
	go build -o $(BINARY) .

install:
	go install .

clean:
	rm -f $(BINARY)

test:
	go test $(PKG)
