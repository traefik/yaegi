// Package pbplug is a plugin that uses google.golang.org/protobuf with its
// own generated messages (pb/event.proto). TestProtobufPlugin runs Report in
// the VM and natively and compares the outputs; BenchmarkProtobuf times
// Roundtrip.
package pbplug

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/traefik/yaegi/v2/vm/testdata/pbplug/pb"
)

// Config is the plugin configuration.
type Config struct{ N int }

// CreateConfig creates the default configuration.
func CreateConfig() *Config { return &Config{N: 50} }

// New creates the middleware: /report writes Report, /roundtrip runs
// Roundtrip N times.
func New(_ context.Context, _ http.Handler, cfg *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/report":
			fmt.Fprint(w, Report())
		case "/roundtrip":
			n := 0
			for i := 0; i < cfg.N; i++ {
				n += Roundtrip()
			}
			fmt.Fprint(w, n)
		}
	}), nil
}

// Sample returns an event that uses every kind of field.
func Sample(n int) *pb.Event {
	retries := int64(3)
	meta, err := structpb.NewStruct(map[string]any{"region": "eu", "weight": 0.5, "tags": []any{"a", true, nil}})
	if err != nil {
		panic(err)
	}
	detail, err := anypb.New(&pb.Header{Name: "detail", Values: []string{"x"}})
	if err != nil {
		panic(err)
	}
	e := &pb.Event{
		Id:      "evt-1",
		Kind:    pb.Kind_KIND_HTTP,
		Retries: &retries,
		Counts:  map[string]int32{"ok": 7, "ko": -2},
		Named:   map[string]*pb.Header{"h": {Name: "host", Values: []string{"example.com"}}},
		Target:  &pb.Event_HeaderTarget{HeaderTarget: &pb.Header{Name: "target"}},
		At:      timestamppb.New(time.Date(2026, 10, 21, 12, 0, 0, 5, time.UTC)),
		Detail:  detail,
		Weights: []float64{1.5, -2, 0},
		Body:    []byte{0, 1, 2, 255},
		Parent:  &pb.Event{Id: "parent", Target: &pb.Event_Url{Url: "https://example.com"}},
		Meta:    meta,
		Kinds:   []pb.Kind{pb.Kind_KIND_TCP, pb.Kind_KIND_HTTP},
	}
	for i := 0; i < n; i++ {
		e.Headers = append(e.Headers, &pb.Header{Name: fmt.Sprint("h", i), Values: []string{"v1", "v2"}})
	}
	return e
}

// Report exercises the API on Sample and returns what it saw, with
// outputs that do not depend on map order.
func Report() string {
	var b strings.Builder
	p := func(a ...any) { fmt.Fprintln(&b, a...) }
	e := Sample(3)

	wire, err := proto.MarshalOptions{Deterministic: true}.Marshal(e)
	p("marshal", len(wire), err, proto.Size(e) == len(wire))
	var back pb.Event
	p("unmarshal", proto.Unmarshal(wire, &back), proto.Equal(e, &back))
	p("fields", back.GetId(), back.GetKind(), back.GetRetries(), back.Retries != nil, len(back.GetHeaders()), back.GetCounts()["ko"],
		back.GetNamed()["h"].GetValues(), back.GetHeaderTarget().GetName(), back.GetUrl() == "", back.GetAt().AsTime(),
		back.GetWeights(), back.GetBody(), back.GetParent().GetUrl(), back.GetMeta().AsMap()["region"], back.GetKinds())

	var d pb.Header
	p("any", back.GetDetail().MessageIs(&d), back.GetDetail().UnmarshalTo(&d), d.GetName(), d.GetValues())

	c := proto.Clone(e).(*pb.Event)
	c.Headers[0].Name = "changed"
	c.Counts["new"] = 1
	p("clone", proto.Equal(c, e), e.Headers[0].Name, len(e.Counts))

	m := &pb.Event{Id: "m", Headers: []*pb.Header{{Name: "first"}}, Counts: map[string]int32{"ok": 1}}
	proto.Merge(m, &pb.Event{Kind: pb.Kind_KIND_TCP, Headers: []*pb.Header{{Name: "second"}}, Counts: map[string]int32{"x": 2}})
	p("merge", m.GetId(), m.GetKind(), len(m.GetHeaders()), m.GetHeaders()[1].GetName(), len(m.GetCounts()))

	reset := proto.Clone(e).(*pb.Event)
	proto.Reset(reset)
	p("reset", reset.GetId() == "", reset.Headers == nil, proto.Size(reset))

	js, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(&pb.Event{Id: "j", Kind: pb.Kind_KIND_TCP, Counts: map[string]int32{"a": 1}, Body: []byte("hi"), Target: &pb.Event_Url{Url: "u"}})
	p("json", err, strings.Contains(string(js), `"kind":"KIND_TCP"`), strings.Contains(string(js), `"body":"aGk="`), strings.Contains(string(js), `"url":"u"`))
	var fromJSON pb.Event
	p("json back", protojson.Unmarshal(js, &fromJSON), fromJSON.GetId(), fromJSON.GetKind(), string(fromJSON.GetBody()), fromJSON.GetUrl())

	txt, err := prototext.Marshal(&pb.Header{Name: "t", Values: []string{"a", "b"}})
	var fromText pb.Header
	p("text", err, prototext.Unmarshal(txt, &fromText), fromText.GetName(), fromText.GetValues())

	// protoreflect: the field names in number order, and a field set by name
	var names []string
	back.ProtoReflect().Range(func(fd protoreflect.FieldDescriptor, _ protoreflect.Value) bool {
		names = append(names, string(fd.Name()))
		return true
	})
	sort.Strings(names)
	p("range", names)
	r := (&pb.Event{}).ProtoReflect()
	r.Set(r.Descriptor().Fields().ByName("id"), protoreflect.ValueOfString("by-reflect"))
	p("reflect set", r.Interface().(*pb.Event).GetId(), r.Descriptor().FullName())

	// a message that is not valid UTF-8 in a string field
	_, err = proto.Marshal(&pb.Header{Name: "\xff"})
	p("utf8", err != nil)
	return b.String()
}

// Roundtrip marshals, unmarshals and compares an event with 20 headers;
// it returns the size of the encoding.
func Roundtrip() int {
	e := Sample(20)
	wire, err := proto.Marshal(e)
	if err != nil {
		panic(err)
	}
	var back pb.Event
	if err := proto.Unmarshal(wire, &back); err != nil {
		panic(err)
	}
	if !proto.Equal(e, &back) {
		panic("not equal")
	}
	return len(wire)
}
