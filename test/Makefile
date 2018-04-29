tests = $(wildcard *.go)

../interp/eval_test.go: $(tests) gen_example.sh
	./gen_example.sh | gofmt > $@
