tests = $(wildcard *.go)

../interp/interp_test.go: $(tests) gen_example.sh
	./gen_example.sh | gofmt > $@
