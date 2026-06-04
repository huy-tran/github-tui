BINARY := gh-tui

# Install directory: GOBIN if set, else $(GOPATH)/bin.
GOBIN := $(shell go env GOBIN)
ifeq ($(GOBIN),)
GOBIN := $(shell go env GOPATH)/bin
endif

# Append .exe on Windows.
ifeq ($(OS),Windows_NT)
EXT := .exe
endif

TARGET := $(GOBIN)/$(BINARY)$(EXT)

.PHONY: all build install run fmt vet tidy clean

all: install

## install: build and place the binary on PATH (GOBIN / GOPATH/bin)
install:
	go build -o "$(TARGET)" .
	@echo "Installed $(TARGET)"

## build: build the binary into the project directory
build:
	go build -o "$(BINARY)$(EXT)" .

## run: build and run locally
run:
	go run .

## fmt: format the source
fmt:
	gofmt -w .

## vet: run go vet
vet:
	go vet ./...

## tidy: tidy go.mod/go.sum
tidy:
	go mod tidy

## clean: remove the locally built binary
clean:
	rm -f "$(BINARY)$(EXT)"
