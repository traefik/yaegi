package dap_test

import (
	"bytes"
	"testing"

	"github.com/traefik/yaegi/internal/dap"
)

func TestCoding(t *testing.T) {
	buf := new(bytes.Buffer)
	enc := dap.NewEncoder(buf)
	dec := dap.NewDecoder(buf)

	err := enc.Encode(&dap.Request{
		Command:   "attach",
		Arguments: &dap.AttachRequestArguments{},
	})
	if err != nil {
		t.Fatal(err)
	}

	msg, err := dec.Decode()
	if err != nil {
		t.Fatal(err)
	}

	rq, ok := msg.(*dap.Request)
	if !ok {
		t.Fatal("Expected request")
	}

	if _, ok := rq.Arguments.(*dap.AttachRequestArguments); !ok {
		t.Fatal("Expected attach request arguments")
	}
}
