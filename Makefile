# Static linting of source files. See .golangci.toml for options
check:
	golangci-lint run

# Generate stdlib/syscall/syscall_GOOS_GOARCH.go for all platforms
gen_all_syscall: internal/cmd/extract/extract
	@for v in $$(go tool dist list); do \
		echo syscall_$${v%/*}_$${v#*/}.go; \
		GOOS=$${v%/*} GOARCH=$${v#*/} go generate ./stdlib/syscall ./stdlib/unrestricted; \
	done

internal/cmd/extract/extract:
	rm -f internal/cmd/extract/extract
	go generate ./internal/cmd/extract

generate: gen_all_syscall
	go generate

install:
	GOFLAGS=-ldflags=-X=main.version=$$(git describe --tags) go install ./...

tests:
	go test -v ./...
	go test -race ./interp

# https://github.com/goreleaser/godownloader
install.sh: .goreleaser.yml
	godownloader --repo=traefik/yaegi -o install.sh .goreleaser.yml

generic_list = cmp/cmp.go slices/slices.go slices/sort.go slices/zsortanyfunc.go maps/maps.go \
			   sync/oncefunc.go sync/atomic/type.go

# get_generic_src imports stdlib files containing generic symbols definitions
get_generic_src:
	eval "`go env`"; echo $$GOROOT; gov=$${GOVERSION#*.}; gov=$${gov%.*}; \
	for f in ${generic_list}; do \
		nf=stdlib/generic/go1_$${gov}_`echo $$f | tr / _`.txt; echo "nf: $$nf"; \
		cat "$$GOROOT/src/$$f" > "$$nf"; \
	done

.PHONY: check gen_all_syscall internal/cmd/extract/extract get_generic_src install
