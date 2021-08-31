package interp

import "strings"

func paramsTypeString(params []*itype) string {
	strs := make([]string, 0, len(params))
	for _, param := range params {
		strs = append(strs, param.str)
	}
	return strings.Join(strs, ",")
}

func methodsTypeString(fields []structField) string {
	strs := make([]string, 0, len(fields))
	for _, field := range fields {
		if field.embed {
			str := methodsTypeString(field.typ.field)
			if str != "" {
				strs = append(strs, str)
			}
			continue
		}
		strs = append(strs, field.name+field.typ.str[4:])
	}
	return strings.Join(strs, "; ")
}

func fieldsTypeString(fields []structField) string {
	strs := make([]string, 0, len(fields))
	for _, field := range fields {
		var repr strings.Builder
		if !field.embed {
			repr.WriteString(field.name)
			repr.WriteByte(' ')
		}
		repr.WriteString(field.typ.str)
		strs = append(strs, repr.String())
	}
	return strings.Join(strs, "; ")
}
