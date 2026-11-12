package graphqlplug

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/formatter"
)

// state validates GraphQL queries against a schema with gqlparser (the
// parser of gqlgen), computes the depth of each query and formats it.
type state struct {
	schema *ast.Schema
}

const schemaSrc = `
type Query {
  user(id: ID!): User
  search(term: String!, first: Int = 10): [Result!]!
}
union Result = User | Post
type User { id: ID! name: String! posts(first: Int): [Post!]! friends: [User!]! }
type Post { id: ID! title: String! author: User! }
`

var queries = map[string]string{
	"/":                `{ user(id: "1") { name } }`,
	"/path/to/page":    `query Q($t: String!) { search(term: $t) { ... on User { name } ... on Post { title author { name } } } }`,
	"/api/data":        `{ user(id: 2) { name friends { friends { friends { posts { title } } } } } }`,
	"/index.html":      `{ user { name } }`,
	"/admin":           `{ __schema { types { name } } }`,
	"/other/malformed": `{ user(id: "1" { name }`,
}

func setup(_ context.Context) (*state, error) {
	schema, err := gqlparser.LoadSchema(&ast.Source{Name: "schema.graphql", Input: schemaSrc})
	if err != nil {
		return nil, err
	}
	return &state{schema: schema}, nil
}

func depth(sel ast.SelectionSet) int {
	d := 0
	for _, s := range sel {
		var sub ast.SelectionSet
		switch x := s.(type) {
		case *ast.Field:
			sub = x.SelectionSet
		case *ast.InlineFragment:
			sub = x.SelectionSet
		case *ast.FragmentSpread:
			sub = x.Definition.SelectionSet
		}
		if n := 1 + depth(sub); n > d {
			d = n
		}
	}
	return d
}

func (s *state) process(req *http.Request) (string, error) {
	q, ok := queries[req.URL.Path]
	if !ok {
		q = queries["/other/malformed"]
	}
	doc, errs := gqlparser.LoadQuery(s.schema, q)
	if len(errs) > 0 {
		var msgs []string
		for _, e := range errs {
			msgs = append(msgs, e.Message)
		}
		return "invalid: " + strings.Join(msgs, "; "), nil
	}
	op := doc.Operations[0]
	var b strings.Builder
	formatter.NewFormatter(&b, formatter.WithIndent("")).FormatQueryDocument(doc)
	return fmt.Sprintf("op=%s depth=%d vars=%d formatted=%q", op.Operation, depth(op.SelectionSet),
		len(op.VariableDefinitions), strings.Join(strings.Fields(b.String()), " ")), nil
}
