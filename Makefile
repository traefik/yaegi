# Static linting of source files. See .golangci.toml for options
check:
	golangci-lint run

# Generate stdlib/syscall/syscall_GOOS_GOARCH.go for all platforms
gen_all_syscall: internal/extract/extract
	@for v in $$(go tool dist list); do \
		echo syscall_$${v%/*}_$${v#*/}.go; \
		GOOS=$${v%/*} GOARCH=$${v#*/} go generate ./stdlib/syscall ./stdlib/unrestricted; \
	done

internal/extract/extract: internal/extract/extract.go
	go generate internal/extract/extract.go

generate: gen_all_syscall
	go generate

tests:
	GO111MODULE=off go test -v ./...
	GO111MODULE=off go test -race ./interp

# https://github.com/goreleaser/godownloader
install.sh: .goreleaser.yml
	godownloader --repo=traefik/yaegi -o install.sh .goreleaser.yml

.PHONY: check gen_all_syscall gen_tests generate_downloader
