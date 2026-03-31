BINARY := bitbucket-mcp

.PHONY: build test run clean

build:
	go build -o $(BINARY) .

test:
	go test ./...

run: build
	BITBUCKET_WORKSPACE=$(BITBUCKET_WORKSPACE) \
	BITBUCKET_USERNAME=$(BITBUCKET_USERNAME) \
	BITBUCKET_APP_PASSWORD=$(BITBUCKET_APP_PASSWORD) \
	./$(BINARY)

clean:
	rm -f $(BINARY)
