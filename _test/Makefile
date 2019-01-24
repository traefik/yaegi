tests = $(wildcard *.go)

generate:
	./gen_example.sh | gofmt > ../interp/interp_test.go

.PHONY: generate