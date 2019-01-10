
check:
	golangci-lint run

gen_all_syscall:
	cd stdlib/syscall && $(MAKE)

.PHONY: check gen_all_syscall
