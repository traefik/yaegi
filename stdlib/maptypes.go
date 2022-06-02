package stdlib

import (
	"encoding"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log"
	"reflect"
)

func init() {
	mt := []reflect.Type{
		reflect.TypeOf((*fmt.Formatter)(nil)).Elem(),
		reflect.TypeOf((*fmt.Stringer)(nil)).Elem(),
	}

	MapTypes[reflect.ValueOf(fmt.Errorf)] = mt
	MapTypes[reflect.ValueOf(fmt.Fprint)] = mt
	MapTypes[reflect.ValueOf(fmt.Fprintf)] = mt
	MapTypes[reflect.ValueOf(fmt.Fprintln)] = mt
	MapTypes[reflect.ValueOf(fmt.Print)] = mt
	MapTypes[reflect.ValueOf(fmt.Printf)] = mt
	MapTypes[reflect.ValueOf(fmt.Println)] = mt
	MapTypes[reflect.ValueOf(fmt.Sprint)] = mt
	MapTypes[reflect.ValueOf(fmt.Sprintf)] = mt
	MapTypes[reflect.ValueOf(fmt.Sprintln)] = mt

	MapTypes[reflect.ValueOf(log.Fatal)] = mt
	MapTypes[reflect.ValueOf(log.Fatalf)] = mt
	MapTypes[reflect.ValueOf(log.Fatalln)] = mt
	MapTypes[reflect.ValueOf(log.Panic)] = mt
	MapTypes[reflect.ValueOf(log.Panicf)] = mt
	MapTypes[reflect.ValueOf(log.Panicln)] = mt

	mt = []reflect.Type{reflect.TypeOf((*fmt.Scanner)(nil)).Elem()}

	MapTypes[reflect.ValueOf(fmt.Scan)] = mt
	MapTypes[reflect.ValueOf(fmt.Scanf)] = mt
	MapTypes[reflect.ValueOf(fmt.Scanln)] = mt

	MapTypes[reflect.ValueOf(json.Marshal)] = []reflect.Type{
		reflect.TypeOf((*json.Marshaler)(nil)).Elem(),
		reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem(),
	}
	MapTypes[reflect.ValueOf(json.Unmarshal)] = []reflect.Type{
		reflect.TypeOf((*json.Unmarshaler)(nil)).Elem(),
		reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem(),
	}
	MapTypes[reflect.ValueOf(xml.Marshal)] = []reflect.Type{
		reflect.TypeOf((*xml.Marshaler)(nil)).Elem(),
		reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem(),
	}
	MapTypes[reflect.ValueOf(xml.Unmarshal)] = []reflect.Type{
		reflect.TypeOf((*xml.Unmarshaler)(nil)).Elem(),
		reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem(),
	}
}
