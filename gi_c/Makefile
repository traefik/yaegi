CPPFLAGS ?= -Wall -Ideps/include
CFLAGS ?= -g
LDFLAGS ?= -Ldeps/lib -llightning

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
