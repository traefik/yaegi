sys = $(shell uname -s)

CPPFLAGS ?= -Wall -Ideps/include
CFLAGS ?= -g

# Linux linker requires rpath set if lib is not in default path
Linux_LDFLAGS = -Wl,-rpath=$(CURDIR)/deps/lib

LDFLAGS ?= $($(sys)_LDFLAGS) -Ldeps/lib -llightning

all: deps gi l2 loop2

gi: gi.o graph.o hash.o init_go.o node.o parse_go.o run_cfg.o run_jit.o scan.o

l2: l2.go
	go build -o l2 l2.go

loop2: loop2.go
	go build -o loop2 loop2.go

test: gi 
	./gi -acx loop2.go

deps: 
	$(MAKE) -C deps

clean:
	rm -f *.o gi loop2 l2

.PHONY: deps clean
