.PHONY: build clean lint release test typecheck

BINARY := conduit
DIST_DIR := dist

build:
	go build -trimpath -o $(DIST_DIR)/$(BINARY) ./cmd/conduit

typecheck:
	go test -run '^$$' ./...

lint:
	test -z "$$(gofmt -l .)"
	go vet ./...

test:
	go test ./...

release: clean
	GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o $(DIST_DIR)/$(BINARY)-darwin-arm64 ./cmd/conduit
	GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o $(DIST_DIR)/$(BINARY)-darwin-amd64 ./cmd/conduit

clean:
	rm -rf $(DIST_DIR)
