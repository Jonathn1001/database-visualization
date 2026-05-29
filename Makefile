.PHONY: dev test build lint clean

# Exclude web/ (npm packages may ship stray .go files, e.g. flatted) from Go tooling.
GO_PKGS = $(shell go list ./... | grep -v /web/)
GO_FILES = $(shell find . -name '*.go' -not -path './web/*')

dev:
	./scripts/dev.sh

test:
	go test -race -cover $(GO_PKGS)
	cd web && npm run test --if-present

build:
	./scripts/build.sh

lint:
	go vet $(GO_PKGS) && gofmt -l $(GO_FILES) | (! grep .)
	cd web && npm run lint

clean:
	rm -rf .air-tmp dist web/node_modules
	find internal/server/dist -type f ! -name '.gitkeep' -delete
