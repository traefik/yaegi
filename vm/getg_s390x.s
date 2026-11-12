#include "textflag.h"

// func getg() uintptr
TEXT ·getg(SB), NOSPLIT, $0-8
	MOVD g, R1
	MOVD R1, ret+0(FP)
	RET
