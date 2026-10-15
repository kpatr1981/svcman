# svcman — Author: Konstantinos Patronas <kpatronas@gmail.com>

PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64
VERSION   ?= 0.1.0
LDFLAGS   := -s -w -X main.version=$(VERSION)

build:
	go build -ldflags="-X main.version=$(VERSION)" -o svcman .

test:
	go vet ./...
	go test ./...

install:
	sh install.sh $(PREFIX)

uninstall:
	sh install.sh --uninstall $(PREFIX)

itest: build
	sh test/lima/itest.sh

release:
	@rm -rf dist && mkdir -p dist
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; ext=; [ $$os = windows ] && ext=.exe; \
		echo "dist/svcman-$$os-$$arch$$ext"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags="$(LDFLAGS)" -o dist/svcman-$$os-$$arch$$ext . || exit 1; \
	done
	@cd dist && shasum -a 256 svcman-* > SHA256SUMS && cat SHA256SUMS

.PHONY: build test install uninstall itest release
