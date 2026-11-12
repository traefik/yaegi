#include "textflag.h"

// func getg() uintptr
TEXT ·getg(SB), NOSPLIT, $0-4
	MOVW g, R0
	MOVW R0, ret+0(FP)
	RET
