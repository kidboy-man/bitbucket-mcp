BINARY := bitbucket-mcp

ifneq (,$(wildcard .env))
	include .env
	export
endif

.PHONY: build test run clean

build:
	go build -o $(BINARY) .

test:
	go test ./...

run: build
	./$(BINARY)

clean:
	rm -f $(BINARY)
