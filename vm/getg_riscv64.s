#include "textflag.h"

// func getg() uintptr
TEXT ·getg(SB), NOSPLIT, $0-8
	MOV g, X10
	MOV X10, ret+0(FP)
	RET
