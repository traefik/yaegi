package wazeroplug

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// state runs a WebAssembly module with wazero (a wasm runtime in pure Go,
// in its interpreter mode): a wasm interpreter inside the Go interpreter.
// The module calls back a Go function of the plugin.
type state struct {
	mu   sync.Mutex
	rt   wazero.Runtime
	mod  api.Module
	fact api.Function
	muls int
}

// factWasm is a module written by hand: it imports env.mul(i32, i32) i32,
// exports a memory and fact(n i32) i32, an iterative factorial that calls
// env.mul for each product.
var factWasm = []byte{
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
	// types: (i32)->i32, (i32,i32)->i32
	0x01, 0x0c, 0x02, 0x60, 0x01, 0x7f, 0x01, 0x7f, 0x60, 0x02, 0x7f, 0x7f, 0x01, 0x7f,
	// import env.mul, type 1
	0x02, 0x0b, 0x01, 0x03, 'e', 'n', 'v', 0x03, 'm', 'u', 'l', 0x00, 0x01,
	// one function of type 0
	0x03, 0x02, 0x01, 0x00,
	// one memory of one page
	0x05, 0x03, 0x01, 0x00, 0x01,
	// exports: fact (function 1), memory
	0x07, 0x11, 0x02, 0x04, 'f', 'a', 'c', 't', 0x00, 0x01, 0x06, 'm', 'e', 'm', 'o', 'r', 'y', 0x02, 0x00,
	// code
	0x0a, 0x28, 0x01, 0x26, 0x01, 0x01, 0x7f,
	0x41, 0x01, 0x21, 0x01, // acc = 1
	0x02, 0x40, 0x03, 0x40, // block loop
	0x20, 0x00, 0x45, 0x0d, 0x01, // if n == 0 break
	0x20, 0x01, 0x20, 0x00, 0x10, 0x00, 0x21, 0x01, // acc = mul(acc, n)
	0x20, 0x00, 0x41, 0x01, 0x6b, 0x21, 0x00, // n--
	0x0c, 0x00, 0x0b, 0x0b, // continue, end loop, end block
	0x20, 0x01, 0x0b, // return acc
}

func setup(ctx context.Context) (*state, error) {
	s := &state{}
	s.rt = wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigInterpreter())
	_, err := s.rt.NewHostModuleBuilder("env").
		NewFunctionBuilder().WithFunc(func(_ context.Context, a, b int32) int32 {
		s.muls++
		return a * b
	}).Export("mul").
		Instantiate(ctx)
	if err != nil {
		return nil, err
	}
	compiled, err := s.rt.CompileModule(ctx, factWasm)
	if err != nil {
		return nil, err
	}
	s.mod, err = s.rt.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName("fact"))
	if err != nil {
		return nil, err
	}
	s.fact = s.mod.ExportedFunction("fact")
	if s.fact == nil {
		return nil, fmt.Errorf("no fact export")
	}
	go func() {
		<-ctx.Done()
		s.rt.Close(context.Background())
	}()
	return s, nil
}

func (s *state) process(req *http.Request) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := uint64(len(strings.Split(strings.Trim(req.URL.Path, "/"), "/")) + 3)
	res, err := s.fact.Call(req.Context(), n)
	if err != nil {
		return "", err
	}
	mem := s.mod.Memory()
	if !mem.Write(16, []byte(req.URL.Path)) {
		return "", fmt.Errorf("memory write")
	}
	back, _ := mem.Read(16, uint32(len(req.URL.Path)))
	return fmt.Sprintf("fact(%d)=%d muls=%d mem=%dB back=%s", n, int32(res[0]), s.muls, mem.Size(), back), nil
}
