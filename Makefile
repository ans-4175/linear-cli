VERSION ?= 0.1.0
PREFIX ?= $(HOME)/.local
LDFLAGS = -s -w -X main.version=$(VERSION)

.PHONY: build test install cross-check

build:
	go build -trimpath -ldflags="$(LDFLAGS)" -o linear-cli .

test:
	go test -race -coverprofile=coverage.out ./...
	go vet ./...

install: build
	mkdir -p "$(PREFIX)/bin"
	install -m 755 linear-cli "$(PREFIX)/bin/linear-cli"

cross-check:
	@mkdir -p dist
	@set -e; for target in darwin/arm64 darwin/amd64 linux/arm64 linux/amd64 windows/arm64 windows/amd64; do \
		os=$${target%/*}; arch=$${target#*/}; suffix=; \
		if [ "$$os" = windows ]; then suffix=.exe; fi; \
		echo "Building $$os/$$arch"; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o "dist/linear-cli-$$os-$$arch$$suffix" .; \
	done
