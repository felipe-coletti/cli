NAME    ?= mycli
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
PKG     := cli/internal/meta
LDFLAGS := -s -w -X $(PKG).Name=$(NAME) -X $(PKG).Version=$(VERSION)
TARGETS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

# Static, stripped binary: no cgo, no local paths embedded.
build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(NAME) .

test:
	go vet ./...
	go test -race ./...

# Cross-compile release binaries into dist/.
release:
	@for target in $(TARGETS); do \
		os=$${target%/*}; arch=$${target#*/}; ext=; \
		if [ "$$os" = windows ]; then ext=.exe; fi; \
		echo "-> dist/$(NAME)-$$os-$$arch$$ext"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "$(LDFLAGS)" \
			-o dist/$(NAME)-$$os-$$arch$$ext . || exit 1; \
	done

clean:
	rm -f $(NAME)
	rm -rf dist

.PHONY: build test release clean
