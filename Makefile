# Static linting of source files. See .golangci.toml for options
check:
	golangci-lint run

# Generate stdlib/syscall/syscall_GOOS_GOARCH.go for all platforms
gen_all_syscall: cmd/goexports/goexports
	@cd stdlib/syscall && \
	for v in $$(go tool dist list); do \
		echo syscall_$${v%/*}_$${v#*/}.go; \
		GOOS=$${v%/*} GOARCH=$${v#*/} go generate; \
	done

cmd/goexports/goexports: cmd/goexports/goexports.go
	go generate cmd/goexports/goexports.go

.PHONY: check gen_all_syscall
