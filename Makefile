.PHONY: build clean gui lint release test tokens tokens-check typecheck

BINARY := conduit
DIST_DIR := dist
DESIGN_DIST := design/dist

build:
	go build -trimpath -o $(DIST_DIR)/$(BINARY) ./cmd/conduit

gui: build
	@trap 'kill $$SERVE_PID 2>/dev/null' EXIT INT TERM; \
	$(DIST_DIR)/$(BINARY) serve & SERVE_PID=$$!; \
	echo "→ backend http://127.0.0.1:9876"; \
	cd spike/gui && npx tauri dev

tokens:
	go run ./cmd/design-tokens

tokens-check: tokens
	@git diff --exit-code -- $(DESIGN_DIST) || \
		(echo "design/dist/ is out of sync with design/tokens.yaml. Run 'make tokens' and commit." && exit 1)

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
