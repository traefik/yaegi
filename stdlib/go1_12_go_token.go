// +build go1.12, !go1.13

package stdlib

// Code generated by 'goexports go/token'. DO NOT EDIT.

import (
	"go/token"
	"reflect"
)

func init() {
	Value["go/token"] = map[string]reflect.Value{
		// function, constant and variable definitions
		"ADD":            reflect.ValueOf(token.ADD),
		"ADD_ASSIGN":     reflect.ValueOf(token.ADD_ASSIGN),
		"AND":            reflect.ValueOf(token.AND),
		"AND_ASSIGN":     reflect.ValueOf(token.AND_ASSIGN),
		"AND_NOT":        reflect.ValueOf(token.AND_NOT),
		"AND_NOT_ASSIGN": reflect.ValueOf(token.AND_NOT_ASSIGN),
		"ARROW":          reflect.ValueOf(token.ARROW),
		"ASSIGN":         reflect.ValueOf(token.ASSIGN),
		"BREAK":          reflect.ValueOf(token.BREAK),
		"CASE":           reflect.ValueOf(token.CASE),
		"CHAN":           reflect.ValueOf(token.CHAN),
		"CHAR":           reflect.ValueOf(token.CHAR),
		"COLON":          reflect.ValueOf(token.COLON),
		"COMMA":          reflect.ValueOf(token.COMMA),
		"COMMENT":        reflect.ValueOf(token.COMMENT),
		"CONST":          reflect.ValueOf(token.CONST),
		"CONTINUE":       reflect.ValueOf(token.CONTINUE),
		"DEC":            reflect.ValueOf(token.DEC),
		"DEFAULT":        reflect.ValueOf(token.DEFAULT),
		"DEFER":          reflect.ValueOf(token.DEFER),
		"DEFINE":         reflect.ValueOf(token.DEFINE),
		"ELLIPSIS":       reflect.ValueOf(token.ELLIPSIS),
		"ELSE":           reflect.ValueOf(token.ELSE),
		"EOF":            reflect.ValueOf(token.EOF),
		"EQL":            reflect.ValueOf(token.EQL),
		"FALLTHROUGH":    reflect.ValueOf(token.FALLTHROUGH),
		"FLOAT":          reflect.ValueOf(token.FLOAT),
		"FOR":            reflect.ValueOf(token.FOR),
		"FUNC":           reflect.ValueOf(token.FUNC),
		"GEQ":            reflect.ValueOf(token.GEQ),
		"GO":             reflect.ValueOf(token.GO),
		"GOTO":           reflect.ValueOf(token.GOTO),
		"GTR":            reflect.ValueOf(token.GTR),
		"HighestPrec":    reflect.ValueOf(token.HighestPrec),
		"IDENT":          reflect.ValueOf(token.IDENT),
		"IF":             reflect.ValueOf(token.IF),
		"ILLEGAL":        reflect.ValueOf(token.ILLEGAL),
		"IMAG":           reflect.ValueOf(token.IMAG),
		"IMPORT":         reflect.ValueOf(token.IMPORT),
		"INC":            reflect.ValueOf(token.INC),
		"INT":            reflect.ValueOf(token.INT),
		"INTERFACE":      reflect.ValueOf(token.INTERFACE),
		"LAND":           reflect.ValueOf(token.LAND),
		"LBRACE":         reflect.ValueOf(token.LBRACE),
		"LBRACK":         reflect.ValueOf(token.LBRACK),
		"LEQ":            reflect.ValueOf(token.LEQ),
		"LOR":            reflect.ValueOf(token.LOR),
		"LPAREN":         reflect.ValueOf(token.LPAREN),
		"LSS":            reflect.ValueOf(token.LSS),
		"Lookup":         reflect.ValueOf(token.Lookup),
		"LowestPrec":     reflect.ValueOf(token.LowestPrec),
		"MAP":            reflect.ValueOf(token.MAP),
		"MUL":            reflect.ValueOf(token.MUL),
		"MUL_ASSIGN":     reflect.ValueOf(token.MUL_ASSIGN),
		"NEQ":            reflect.ValueOf(token.NEQ),
		"NOT":            reflect.ValueOf(token.NOT),
		"NewFileSet":     reflect.ValueOf(token.NewFileSet),
		"NoPos":          reflect.ValueOf(token.NoPos),
		"OR":             reflect.ValueOf(token.OR),
		"OR_ASSIGN":      reflect.ValueOf(token.OR_ASSIGN),
		"PACKAGE":        reflect.ValueOf(token.PACKAGE),
		"PERIOD":         reflect.ValueOf(token.PERIOD),
		"QUO":            reflect.ValueOf(token.QUO),
		"QUO_ASSIGN":     reflect.ValueOf(token.QUO_ASSIGN),
		"RANGE":          reflect.ValueOf(token.RANGE),
		"RBRACE":         reflect.ValueOf(token.RBRACE),
		"RBRACK":         reflect.ValueOf(token.RBRACK),
		"REM":            reflect.ValueOf(token.REM),
		"REM_ASSIGN":     reflect.ValueOf(token.REM_ASSIGN),
		"RETURN":         reflect.ValueOf(token.RETURN),
		"RPAREN":         reflect.ValueOf(token.RPAREN),
		"SELECT":         reflect.ValueOf(token.SELECT),
		"SEMICOLON":      reflect.ValueOf(token.SEMICOLON),
		"SHL":            reflect.ValueOf(token.SHL),
		"SHL_ASSIGN":     reflect.ValueOf(token.SHL_ASSIGN),
		"SHR":            reflect.ValueOf(token.SHR),
		"SHR_ASSIGN":     reflect.ValueOf(token.SHR_ASSIGN),
		"STRING":         reflect.ValueOf(token.STRING),
		"STRUCT":         reflect.ValueOf(token.STRUCT),
		"SUB":            reflect.ValueOf(token.SUB),
		"SUB_ASSIGN":     reflect.ValueOf(token.SUB_ASSIGN),
		"SWITCH":         reflect.ValueOf(token.SWITCH),
		"TYPE":           reflect.ValueOf(token.TYPE),
		"UnaryPrec":      reflect.ValueOf(token.UnaryPrec),
		"VAR":            reflect.ValueOf(token.VAR),
		"XOR":            reflect.ValueOf(token.XOR),
		"XOR_ASSIGN":     reflect.ValueOf(token.XOR_ASSIGN),

		// type definitions
		"File":     reflect.ValueOf((*token.File)(nil)),
		"FileSet":  reflect.ValueOf((*token.FileSet)(nil)),
		"Pos":      reflect.ValueOf((*token.Pos)(nil)),
		"Position": reflect.ValueOf((*token.Position)(nil)),
		"Token":    reflect.ValueOf((*token.Token)(nil)),
	}
}
