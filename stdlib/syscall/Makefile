# Generate syscall_GOOS_GOARCH.go for all combinations
all: gen_all

gen_all: ../../cmd/goexports/goexports
	@for v in $$(go tool dist list); do \
		echo syscall_$${v%/*}_$${v#*/}.go; \
		GOOS=$${v%/*} GOARCH=$${v#*/} go generate; \
	done

# To check that all platforms syscall files compile correctly
build_all:
	@for v in $$(go tool dist list); do \
		echo syscall_$${v%/*}_$${v#*/}.go; \
		GOOS=$${v%/*} GOARCH=$${v#*/} go build; \
	done

../../cmd/goexports/goexports: ../../cmd/goexports/goexports.go
	go generate ../../cmd/goexports

.PHONY = gen_all build_all
