package main

import (
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func main() {
	s, err := structpb.NewStruct(map[string]any{"name": "traefik", "n": 3, "tags": []any{"a", "b"}})
	fmt.Println(err)
	b, err := proto.MarshalOptions{Deterministic: true}.Marshal(s)
	fmt.Println(len(b), err)
	var s2 structpb.Struct
	fmt.Println(proto.Unmarshal(b, &s2), proto.Equal(s, &s2))
	fmt.Println(s2.Fields["name"].GetStringValue(), s2.AsMap()["n"])
	w := wrapperspb.String("hello")
	wb, _ := proto.Marshal(w)
	fmt.Println(wb)
}
