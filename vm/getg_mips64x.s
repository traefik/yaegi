//go:build mips64 || mips64le

#include "textflag.h"

// func getg() uintptr
TEXT ·getg(SB), NOSPLIT, $0-8
	MOVV g, R1
	MOVV R1, ret+0(FP)
	RET
