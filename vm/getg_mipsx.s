//go:build mips || mipsle

#include "textflag.h"

// func getg() uintptr
TEXT ·getg(SB), NOSPLIT, $0-4
	MOVW g, R1
	MOVW R1, ret+0(FP)
	RET
