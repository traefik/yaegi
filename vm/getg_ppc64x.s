//go:build ppc64 || ppc64le

#include "textflag.h"

// func getg() uintptr
TEXT ·getg(SB), NOSPLIT, $0-8
	MOVD g, R3
	MOVD R3, ret+0(FP)
	RET
