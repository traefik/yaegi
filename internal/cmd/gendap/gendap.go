package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/traefik/yaegi/internal/jsonx"
)

var schemaUrl = flag.String("url", "", "URL of the schema")
var packageName = flag.String("name", "", "Package name")
var filePath = flag.String("path", "", "File to write to")
var jsonPatch = flag.String("patch", "", "JSON file to apply as a patch")

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format, args...)
	os.Exit(1)
}

func main() {
	flag.Parse()

	if *schemaUrl == "" {
		fatalf("--url is required")
	}
	if *packageName == "" {
		fatalf("--name is required")
	}
	if *filePath == "" {
		fatalf("--path is required")
	}

	resp, err := http.Get(*schemaUrl)
	if err != nil {
		fatalf("http: %v\n", err)
	}

	if resp.StatusCode != 200 {
		resp.Body.Close()
		fatalf("http: expected status 200, got %d\n", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		fatalf("read (schema): %v\n", err)
	}

	if *jsonPatch != "" {
		b, err = jsonx.ApplyPatch(b, *jsonPatch)
		if err != nil {
			fatalf("%v\n", err)
		}
	}

	var schema jsonx.Schema
	err = json.Unmarshal(b, &schema)
	if err != nil {
		fatalf("json (schema): %v\n", err)
	}

	var requests, responses, events, missing Types
	for name, s := range schema.Definitions {
		if len(s.AllOf) != 2 || s.AllOf[0].Ref == "" {
			continue
		}

		switch s.AllOf[0].Ref {
		case "#/definitions/Request":
			cmd := s.AllOf[1].Properties["command"]
			if cmd == nil || len(cmd.Enum) != 1 {
				continue
			}

			if cmd.Enum[0] == "configurationDone" {
				println("!")
			}

			typ := Type{
				Identifier: cmd.Enum[0],
				Name:       name[:len(name)-len("Request")] + "Arguments",
			}
			if schema.Definitions[typ.Name] != nil {
				// ok
			} else if schema.Definitions[name+"Arguments"] != nil {
				typ.Name = name + "Arguments"
			} else {
				missing = append(missing, typ)
			}

			requests = append(requests, typ)

		case "#/definitions/Response":
			id := name[:len(name)-len("Response")]
			id = strings.ToLower(id[:1]) + id[1:]
			typ := Type{
				Identifier: id,
				Name:       name + "Body",
			}

			if schema.Definitions[typ.Name] != nil {
				// ok
			} else if id == "initialize" && schema.Definitions["Capabilities"] != nil {
				typ.Name = "Capabilities"
			} else {
				missing = append(missing, typ)
			}

			responses = append(responses, typ)

		case "#/definitions/Event":
			evt := s.AllOf[1].Properties["event"]
			if evt == nil || len(evt.Enum) != 1 {
				continue
			}
			typ := Type{
				Identifier: evt.Enum[0],
				Name:       name + "Body",
			}

			if schema.Definitions[typ.Name] == nil {
				missing = append(missing, typ)
			}

			events = append(events, typ)
		}
	}

	requests.Sort()
	responses.Sort()
	events.Sort()
	missing.Sort()

	w := new(bytes.Buffer)
	fmt.Fprintf(w, "package %s\n", *packageName)
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "// Code generated by 'go run ../internal/cmd/gendap'. DO NOT EDIT.\n")
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "import \"fmt\"\n")
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "type RequestArguments interface{ requestType() string }\n")
	fmt.Fprintf(w, "type ResponseBody interface{ responseType() string }\n")
	fmt.Fprintf(w, "type EventBody interface{ eventType() string }\n")
	fmt.Fprintf(w, "\n")

	requests.writeFuncs(w, "requestType")
	responses.writeFuncs(w, "responseType")
	events.writeFuncs(w, "eventType")
	missing.writeTypes(w)

	requests.writeConstructor(w, "newRequest", "RequestArguments", "unrecognized command %q")
	responses.writeConstructor(w, "newResponse", "ResponseBody", "unrecognized command %q")
	events.writeConstructor(w, "newEvent", "EventBody", "unrecognized event %q")

	f, err := os.Create(*filePath)
	if err != nil {
		fatalf("json: %v\n", err)
	}
	defer f.Close()

	// w.WriteTo(f)

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, *filePath, w, parser.ParseComments)
	if err != nil {
		fatalf("json: %v\n", err)
	}

	err = format.Node(f, fset, file)
	if err != nil {
		fatalf("json: %v\n", err)
	}
}

type Type struct {
	Name, Identifier string
}

type Types []Type

func (t Types) Less(i, j int) bool { return t[i].Name < t[j].Name }
func (t Types) Sort()              { sort.Slice(t, t.Less) }

func (t Types) writeFuncs(w io.Writer, name string) {
	for _, t := range t {
		fmt.Fprintf(w, "func (*%s) %s() string { return %q }\n", t.Name, name, t.Identifier)
	}
	fmt.Fprintf(w, "\n")
}

func (t Types) writeTypes(w io.Writer) {
	for _, t := range t {
		fmt.Fprintf(w, "type %s struct{}\n", t.Name)
	}
	fmt.Fprintf(w, "\n")
}

func (t Types) writeConstructor(w io.Writer, name, typ, err string) {
	fmt.Fprintf(w, "func %s(x string) (%s, error) {\n", name, typ)
	fmt.Fprintf(w, "\tswitch x {\n")
	for _, t := range t {
		fmt.Fprintf(w, "\tcase %q:\n", t.Identifier)
		fmt.Fprintf(w, "\t\treturn new(%s), nil\n", t.Name)
	}
	fmt.Fprintf(w, "\tdefault:\n")
	fmt.Fprintf(w, "\t\treturn nil, fmt.Errorf(%q, x)\n", err)
	fmt.Fprintf(w, "\t}\n")
	fmt.Fprintf(w, "}\n")
	fmt.Fprintf(w, "\n")
}
