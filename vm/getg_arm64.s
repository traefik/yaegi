#include "textflag.h"

// func getg() uintptr
TEXT ·getg(SB), NOSPLIT, $0-8
	MOVD g, R0
	MOVD R0, ret+0(FP)
	RET
