//go:build ignore

// gen writes event.pb.go: the descriptor of event.proto, built in Go (there
// is no protoc in the development setup), given to the generator of
// protoc-gen-go. Run it from vm/testdata/pbplug/pb: go run gen/main.go
package main

import (
	"os"
	"path/filepath"
	"strings"

	gengo "google.golang.org/protobuf/cmd/protoc-gen-go/internal_gengo"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/pluginpb"
)

type (
	fd = descriptorpb.FieldDescriptorProto
	ty = descriptorpb.FieldDescriptorProto_Type
)

const (
	optional = descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	repeated = descriptorpb.FieldDescriptorProto_LABEL_REPEATED
	tString  = descriptorpb.FieldDescriptorProto_TYPE_STRING
	tInt64   = descriptorpb.FieldDescriptorProto_TYPE_INT64
	tInt32   = descriptorpb.FieldDescriptorProto_TYPE_INT32
	tDouble  = descriptorpb.FieldDescriptorProto_TYPE_DOUBLE
	tBytes   = descriptorpb.FieldDescriptorProto_TYPE_BYTES
	tEnum    = descriptorpb.FieldDescriptorProto_TYPE_ENUM
	tMessage = descriptorpb.FieldDescriptorProto_TYPE_MESSAGE
)

// jsonName is the default JSON name protoc gives a field (lowerCamelCase).
func jsonName(s string) string {
	parts := strings.Split(s, "_")
	for i := 1; i < len(parts); i++ {
		if parts[i] != "" {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

func field(name string, num int32, label descriptorpb.FieldDescriptorProto_Label, t ty, typeName string) *fd {
	f := &fd{Name: proto.String(name), Number: proto.Int32(num), Label: label.Enum(), Type: t.Enum(), JsonName: proto.String(jsonName(name))}
	if typeName != "" {
		f.TypeName = proto.String(typeName)
	}
	return f
}

// mapEntry is the nested message protoc makes for a map field.
func mapEntry(name string, vt ty, vtype string) *descriptorpb.DescriptorProto {
	return &descriptorpb.DescriptorProto{
		Name: proto.String(name),
		Field: []*fd{
			field("key", 1, optional, tString, ""),
			field("value", 2, optional, vt, vtype),
		},
		Options: &descriptorpb.MessageOptions{MapEntry: proto.Bool(true)},
	}
}

func main() {
	retries := field("retries", 3, optional, tInt64, "")
	retries.OneofIndex, retries.Proto3Optional = proto.Int32(1), proto.Bool(true)
	url := field("url", 7, optional, tString, "")
	url.OneofIndex = proto.Int32(0)
	ht := field("header_target", 8, optional, tMessage, ".pbplug.Header")
	ht.OneofIndex = proto.Int32(0)

	file := &descriptorpb.FileDescriptorProto{
		Name:       proto.String("event.proto"),
		Package:    proto.String("pbplug"),
		Syntax:     proto.String("proto3"),
		Dependency: []string{"google/protobuf/any.proto", "google/protobuf/struct.proto", "google/protobuf/timestamp.proto"},
		Options:    &descriptorpb.FileOptions{GoPackage: proto.String("github.com/traefik/yaegi/v2/vm/testdata/pbplug/pb")},
		EnumType: []*descriptorpb.EnumDescriptorProto{{
			Name: proto.String("Kind"),
			Value: []*descriptorpb.EnumValueDescriptorProto{
				{Name: proto.String("KIND_UNSPECIFIED"), Number: proto.Int32(0)},
				{Name: proto.String("KIND_HTTP"), Number: proto.Int32(1)},
				{Name: proto.String("KIND_TCP"), Number: proto.Int32(2)},
			},
		}},
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("Header"),
				Field: []*fd{
					field("name", 1, optional, tString, ""),
					field("values", 2, repeated, tString, ""),
				},
			},
			{
				Name: proto.String("Event"),
				Field: []*fd{
					field("id", 1, optional, tString, ""),
					field("kind", 2, optional, tEnum, ".pbplug.Kind"),
					retries,
					field("headers", 4, repeated, tMessage, ".pbplug.Header"),
					field("counts", 5, repeated, tMessage, ".pbplug.Event.CountsEntry"),
					field("named", 6, repeated, tMessage, ".pbplug.Event.NamedEntry"),
					url,
					ht,
					field("at", 9, optional, tMessage, ".google.protobuf.Timestamp"),
					field("detail", 10, optional, tMessage, ".google.protobuf.Any"),
					field("weights", 11, repeated, tDouble, ""),
					field("body", 12, optional, tBytes, ""),
					field("parent", 13, optional, tMessage, ".pbplug.Event"),
					field("meta", 14, optional, tMessage, ".google.protobuf.Struct"),
					field("kinds", 15, repeated, tEnum, ".pbplug.Kind"),
				},
				NestedType: []*descriptorpb.DescriptorProto{
					mapEntry("CountsEntry", tInt32, ""),
					mapEntry("NamedEntry", tMessage, ".pbplug.Header"),
				},
				OneofDecl: []*descriptorpb.OneofDescriptorProto{
					{Name: proto.String("target")},
					{Name: proto.String("_retries")},
				},
			},
		},
	}

	req := &pluginpb.CodeGeneratorRequest{
		FileToGenerate: []string{"event.proto"},
		ProtoFile: []*descriptorpb.FileDescriptorProto{
			protodesc.ToFileDescriptorProto(anypb.File_google_protobuf_any_proto),
			protodesc.ToFileDescriptorProto(structpb.File_google_protobuf_struct_proto),
			protodesc.ToFileDescriptorProto(timestamppb.File_google_protobuf_timestamp_proto),
			file,
		},
	}
	plugin, err := protogen.Options{}.New(req)
	if err != nil {
		panic(err)
	}
	plugin.SupportedFeatures = gengo.SupportedFeatures
	for _, f := range plugin.Files {
		if f.Generate {
			gengo.GenerateFile(plugin, f)
		}
	}
	resp := plugin.Response()
	if resp.Error != nil {
		panic(*resp.Error)
	}
	for _, f := range resp.File {
		if err := os.WriteFile(filepath.Base(f.GetName()), []byte(f.GetContent()), 0o644); err != nil {
			panic(err)
		}
	}
}
