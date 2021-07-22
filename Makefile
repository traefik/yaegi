GO?=go

# Static linting of source files. See .golangci.toml for options
check:
	golangci-lint run

# Generate stdlib/syscall/syscall_GOOS_GOARCH.go for all platforms
gen_all_syscall: internal/cmd/extract/extract
	@for v in $$($(GO) tool dist list); do \
		echo syscall_$${v%/*}_$${v#*/}.go; \
		GOOS=$${v%/*} GOARCH=$${v#*/} $(GO) generate ./stdlib/syscall ./stdlib/unrestricted; \
	done

internal/cmd/extract/extract:
	rm -f internal/cmd/extract/extract
	$(GO) generate ./internal/cmd/extract

generate: gen_all_syscall
	$(GO) generate

install:
	GOFLAGS=-ldflags=-X=main.version=$$(git describe --tags) $(GO) install ./...

tests:
	$(GO) test -v ./...
	$(GO) test -race ./interp

# https://github.com/goreleaser/godownloader
install.sh: .goreleaser.yml
	godownloader --repo=traefik/yaegi -o install.sh .goreleaser.yml

.PHONY: check gen_all_syscall gen_tests generate_downloader internal/cmd/extract/extract install
