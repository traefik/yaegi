package vm

// golang.org/x/sys/cpu run from source.
//
// x/sys/cpu tells which extensions the CPU has (AVX2, AES, SHA...). Its init
// reads them with small functions written in assembly: cpuid and xgetbv on
// 386 and amd64, getisar0, getisar1, getpfr0 and getzfr0 on arm64,
// readVLENB on riscv64, stfle and the kmQuery... functions on s390x,
// get_cpucfg on loong64, and a sysctl through the C library on darwin and
// openbsd (syscall_syscall6). The VM cannot run assembly, so a plugin that
// imports x/sys/cpu (validator through golang.org/x/crypto/sha3, wazero,
// crypto libraries) was refused at load time, unless the host set
// Options.StubAssembly. On linux/amd64 this hit every validator plugin
// whose validator did not come from the AOT copy: another version, or a
// host built with a newer Go patch release than the copies.
//
// The VM gives these functions to every plugin with one answer: zero, a
// CPU without any extension (cpuid(0, 0) says that no other leaf exists).
// The sysctl of darwin and openbsd fails with ENOSYS without calling the
// system, so no feature is seen there either. The init then leaves every
// feature variable false, as on an old CPU, and the libraries take their
// pure Go code, which the VM runs. This is the only answer that is safe:
// the libraries check these variables to choose their assembly, which the
// VM cannot run, so a true feature (the host's real CPU) would lead an
// interpreted library to a function in assembly. A plugin that prints the
// features sees false where go run sees true; the results of the
// libraries are the same.
//
// The functions are found by package path, name and signature: only
// functions without a Go body of golang.org/x/sys/cpu run from source,
// whose parameters and results are integers or plain data made of
// integers (no pointer: the VM reads and writes nothing).

import (
	"go/types"

	"golang.org/x/tools/go/ssa"
)

const xsysCPUPath = "golang.org/x/sys/cpu"

// xsysCPUZero are the functions of golang.org/x/sys/cpu written in assembly
// that the VM gives as returning zeros.
var xsysCPUZero = map[string]bool{
	// 386, amd64
	"cpuid": true, "xgetbv": true,
	// arm64
	"getisar0": true, "getisar1": true, "getpfr0": true, "getzfr0": true,
	// riscv64
	"readVLENB": true,
	// s390x
	"stfle": true, "kmQuery": true, "kmcQuery": true, "kmctrQuery": true,
	"kmaQuery": true, "kimdQuery": true, "klmdQuery": true,
	// loong64
	"get_cpucfg": true,
}

// xsysCPUHook returns the function that the VM runs for f, a function
// without a Go body of a package run from source, when f is one of the
// assembly functions of golang.org/x/sys/cpu (see the top of this file).
// nil otherwise.
func (m *Machine) xsysCPUHook(f *ssa.Function) NativeHook {
	if f.Pkg.Pkg.Path() != xsysCPUPath || f.TypeParams().Len() > 0 {
		return nil
	}
	sig := f.Signature
	for i := 0; i < sig.Params().Len(); i++ {
		if !plainInts(sig.Params().At(i).Type()) {
			return nil
		}
	}
	for i := 0; i < sig.Results().Len(); i++ {
		if !plainInts(sig.Results().At(i).Type()) {
			return nil
		}
	}
	switch {
	case xsysCPUZero[f.Name()] && sig.Results().Len() > 0:
		res := sig.Results()
		return func(m *Machine, a []Value) Value { return m.zeroTuple(res) }
	case f.Name() == "syscall_syscall6" && libcSyscalls[m.linkTarget(f)] && sig.Results().Len() == 3:
		// The sysctl of darwin and openbsd: it fails, as on a system
		// without it, and nothing is called.
		return enosysHook
	}
	return nil
}

// plainInts reports whether t is an integer, or an array or a struct made
// only of integers.
func plainInts(t types.Type) bool {
	switch u := t.Underlying().(type) {
	case *types.Basic:
		return u.Info()&types.IsInteger != 0 && u.Kind() != types.UnsafePointer
	case *types.Array:
		return plainInts(u.Elem())
	case *types.Struct:
		for i := 0; i < u.NumFields(); i++ {
			if !plainInts(u.Field(i).Type()) {
				return false
			}
		}
		return true
	}
	return false
}
