package protobufplug

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"example.com/complex/protobuf/eventpb"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// state uses generated messages (eventpb, made by protoc-gen-go), the
// well-known types, the JSON and text formats, Any and dynamic messages.
type state struct {
	n int
}

func setup(_ context.Context) (*state, error) {
	// The descriptor of the generated file must be in the global registry.
	if _, err := protoregistry.GlobalFiles.FindFileByPath("complex/v1/event.proto"); err != nil {
		return nil, err
	}
	return &state{}, nil
}

func (s *state) process(req *http.Request) (string, error) {
	s.n++
	ev := &eventpb.Event{
		Id:     req.Method + " " + req.URL.Path,
		Ts:     int64(s.n),
		Labels: map[string]string{"host": req.Host, "ua": req.UserAgent()},
		Kind:   eventpb.Kind_KIND_READ,
		At:     timestamppb.New(timestampOf(s.n)),
	}
	if req.Method != http.MethodGet {
		ev.Kind = eventpb.Kind_KIND_WRITE
	}
	for _, k := range sortedKeys(req.URL.Query()) {
		ev.Items = append(ev.Items, &eventpb.Item{Name: k, Price: float64(len(k)) * 1.5, Qty: int32(len(req.URL.Query()[k]))})
	}
	if req.Body != nil && req.ContentLength > 0 {
		ev.Payload = &eventpb.Event_Raw{Raw: []byte("body")}
	} else {
		ev.Payload = &eventpb.Event_Text{Text: "no body"}
	}

	// Binary round trip.
	b, err := proto.MarshalOptions{Deterministic: true}.Marshal(ev)
	if err != nil {
		return "", err
	}
	var back eventpb.Event
	if err := proto.Unmarshal(b, &back); err != nil {
		return "", err
	}
	if !proto.Equal(ev, &back) {
		return "", fmt.Errorf("round trip changed the message")
	}

	// JSON, with the names of the proto file.
	js, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(&back)
	if err != nil {
		return "", err
	}
	var fromJSON eventpb.Event
	if err := protojson.Unmarshal(js, &fromJSON); err != nil {
		return "", err
	}

	// Any and Struct.
	a, err := anypb.New(&fromJSON)
	if err != nil {
		return "", err
	}
	m, err := a.UnmarshalNew()
	if err != nil {
		return "", err
	}
	st, err := structpb.NewStruct(map[string]any{"path": req.URL.Path, "n": s.n, "tags": []any{"a", true}})
	if err != nil {
		return "", err
	}

	// A dynamic message of the same type, through reflection.
	md := ev.ProtoReflect().Descriptor()
	dm := dynamicpb.NewMessage(md)
	if err := proto.Unmarshal(b, dm); err != nil {
		return "", err
	}
	var fields []string
	dm.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		fields = append(fields, string(fd.Name()))
		return true
	})
	sort.Strings(fields)

	txt := prototext.MarshalOptions{}.Format(&eventpb.Item{Name: "x", Qty: 3})
	txt = strings.Join(strings.Fields(txt), " ")

	return fmt.Sprintf("bin=%d sha=%s any=%s kind=%s items=%d dyn=%v struct=%d text=%q json=%s",
		len(b), base64.RawStdEncoding.EncodeToString(b[:min(6, len(b))]), a.GetTypeUrl(),
		m.(*eventpb.Event).GetKind(), len(fromJSON.GetItems()), fields, len(st.GetFields()), txt, normJSON(js)), nil
}

func timestampOf(n int) time.Time {
	return time.Unix(1_700_000_000+int64(n), 123_000_000).UTC()
}

func sortedKeys(m map[string][]string) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// normJSON removes the spaces that protojson adds at random to stop people
// from comparing its output byte for byte.
func normJSON(b []byte) string {
	return strings.ReplaceAll(strings.ReplaceAll(string(b), " ", ""), "\t", "")
}
