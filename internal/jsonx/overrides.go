package jsonx

import (
	"encoding/json"
)

type Schema_Type []SimpleTypes

func (t *Schema_Type) UnmarshalJSON(b []byte) error {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}

	switch v := v.(type) {
	case nil:
		*t = nil
	case string:
		*t = Schema_Type{SimpleTypes(v)}
	case []string:
		*t = make(Schema_Type, len(v))
		for i := range v {
			(*t)[i] = SimpleTypes(v[i])
		}
	default:
		return json.Unmarshal(b, (*[]SimpleTypes)(t))
	}
	return nil
}

func (t Schema_Type) MarshalJSON() ([]byte, error) {
	if len(t) == 1 {
		return json.Marshal(t[0])
	}
	return json.Marshal([]SimpleTypes(t))
}
