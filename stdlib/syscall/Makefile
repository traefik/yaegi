# Generate syscall_GOOS_GOARCH.go for all combinations
all: gen_all

gen_all:
	@for v in $$(go tool dist list); do \
		echo syscall_$${v%/*}_$${v#*/}.go; \
		GOOS=$${v%/*} GOARCH=$${v#*/} go generate; \
	done

build_all:
	@for v in $$(go tool dist list); do \
		echo syscall_$${v%/*}_$${v#*/}.go; \
		GOOS=$${v%/*} GOARCH=$${v#*/} go build; \
	done

.PHONY = gen_all build_all
