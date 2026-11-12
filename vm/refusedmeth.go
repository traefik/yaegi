package vm

// The refused methods at run time. The load-time check (restrict.go) sees
// only the functions that the code of the plugin names (types.Info.Uses).
// A plugin can call a method of a native value without naming it:
//
//   - through its reflect (the twin): Value.Method, Value.MethodByName,
//     Type.Method, Type.MethodByName, then Call or Interface;
//   - through an interface of its own (interface{ Chdir() error }), or a
//     method that a struct type of the plugin promotes from an embedded
//     native field;
//   - through text/template and html/template, which call the methods of
//     their data by name ({{.Chdir}}) with the native reflect.
//
// A restricted machine (m.restricted) gets, for each refused method of a
// native type, a function that panics instead (refusedMethods). The VM
// finds it where it finds the replacements of exit.go: lookupMethod for
// static calls, method values and calls through an interface, and
// restrictedMethodValue and restrictedMethodFunc for the twin of reflect.
// The AOT copies of text/template and html/template use the twin, so they
// are covered too. The native templates call the native reflect, which the
// VM does not see: for them, Execute and ExecuteTemplate run a private
// copy of the templates (textCopy, htmlCopy), and refuse it when it names
// a refused method or an Execute method (templateRefused).
//
// The refusal is by name in templates: a field of the data with the name
// of a refused method (a struct field Chdir or Execute) is refused too. The
// names are rare (Chdir, Execute and ExecuteTemplate today).
//
// TestRefusedMethods checks each path, and that every method of
// refusedFuncs has an entry here.

import (
	"fmt"
	htmltemplate "html/template"
	"io"
	"maps"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"text/template"
	"text/template/parse"
)

// refusedNative are the native methods of refusedFuncs, by type and name,
// with their types.Func.FullName.
var refusedNative = map[methodKey]string{
	{reflect.TypeOf((*os.File)(nil)), "Chdir"}: "(*os.File).Chdir",
}

// refusedMethods are the functions that a restricted machine gets instead
// of the methods of refusedNative (methodFunc: the receiver comes first),
// and the replacements of the Execute methods of the native templates.
var refusedMethods = sync.OnceValue(func() map[methodKey]reflect.Value {
	refused := map[methodKey]reflect.Value{}
	for k, what := range refusedNative {
		meth, ok := k.t.MethodByName(k.name)
		if !ok {
			panic("vm: no method " + what)
		}
		refused[k] = reflect.MakeFunc(meth.Func.Type(), func([]reflect.Value) []reflect.Value {
			panic(&RefusedError{What: what})
		})
	}
	tt := reflect.TypeOf((*template.Template)(nil))
	ht := reflect.TypeOf((*htmltemplate.Template)(nil))
	refused[methodKey{tt, "Execute"}] = reflect.ValueOf(textExecute)
	refused[methodKey{tt, "ExecuteTemplate"}] = reflect.ValueOf(textExecuteTemplate)
	refused[methodKey{ht, "Execute"}] = reflect.ValueOf(htmlExecute)
	refused[methodKey{ht, "ExecuteTemplate"}] = reflect.ValueOf(htmlExecuteTemplate)
	hostFDMethods(refused) // http.Dir.Open, (*os.Root).OpenRoot (hostfd.go)
	return refused
})

// refusedNames are the names of the methods that a template may not call:
// the methods of refusedNative, and the Execute methods of the templates.
var refusedNames = sync.OnceValue(func() map[string]bool {
	// Execute and ExecuteTemplate too: a template that calls the Execute
	// method of a native template in its data ({{.T.Execute .W .F}}) runs
	// it with the native method, without the check.
	names := map[string]bool{"Execute": true, "ExecuteTemplate": true}
	for k := range refusedNative {
		names[k.name] = true
	}
	return names
})

// RefusedError is the panic of a refused method that a restricted plugin
// calls at run time.
type RefusedError struct {
	What string // the method, as types.Func.FullName
}

func (e *RefusedError) Error() string {
	return fmt.Sprintf("%s is not allowed in a plugin", e.What)
}

// refusedMethodOf returns the replacement of the method name of the native
// type t for the restricted machine m. m is nil for a goroutine that runs
// no interpreted code (the twin of reflect called by an AOT copy on a
// goroutine of native code): the method is refused too.
func refusedMethodOf(m *Machine, t reflect.Type, name string) (reflect.Value, bool) {
	if m != nil && !m.restricted {
		return reflect.Value{}, false
	}
	fn, ok := refusedMethods()[methodKey{t, name}]
	return fn, ok
}

// templateRefused returns an error when one of trees names a refused
// method: a field ({{.Chdir}}), a field of a variable ({{$x.Chdir}}) or of
// a chain ({{(.F).Chdir}}).
func templateRefused(trees []*parse.Tree) error {
	names := refusedNames()
	if execNamesRefused(curMachine()) {
		// ExecMock: a *exec.Cmd in the data would start its process, or
		// read the environment of the host
		names = maps.Clone(names)
		for _, n := range execTemplateNames {
			names[n] = true
		}
	}
	var found []string
	var walk func(n parse.Node)
	idents := func(ids []string) {
		for _, id := range ids {
			if names[id] {
				found = append(found, id)
			}
		}
	}
	pipe := func(p *parse.PipeNode) {
		if p != nil {
			walk(p)
		}
	}
	list := func(l *parse.ListNode) {
		if l != nil {
			walk(l)
		}
	}
	walk = func(n parse.Node) {
		switch n := n.(type) {
		case *parse.ListNode:
			for _, c := range n.Nodes {
				walk(c)
			}
		case *parse.ActionNode:
			pipe(n.Pipe)
		case *parse.PipeNode:
			for _, c := range n.Cmds {
				walk(c)
			}
		case *parse.CommandNode:
			for _, a := range n.Args {
				walk(a)
			}
		case *parse.FieldNode:
			idents(n.Ident)
		case *parse.VariableNode:
			idents(n.Ident[1:])
		case *parse.ChainNode:
			walk(n.Node)
			idents(n.Field)
		case *parse.IfNode:
			pipe(n.Pipe)
			list(n.List)
			list(n.ElseList)
		case *parse.RangeNode:
			pipe(n.Pipe)
			list(n.List)
			list(n.ElseList)
		case *parse.WithNode:
			pipe(n.Pipe)
			list(n.List)
			list(n.ElseList)
		case *parse.TemplateNode:
			pipe(n.Pipe)
		case *parse.TextNode, *parse.CommentNode, *parse.IdentifierNode, *parse.DotNode, *parse.NilNode,
			*parse.BoolNode, *parse.NumberNode, *parse.StringNode, *parse.BreakNode, *parse.ContinueNode:
		default:
			// A node of a new Go version: look for the names in its text.
			s := n.String()
			for name := range names {
				if strings.Contains(s, name) {
					found = append(found, name)
				}
			}
		}
	}
	for _, t := range trees {
		if t != nil && t.Root != nil {
			walk(t.Root)
		}
	}
	if len(found) == 0 {
		return nil
	}
	sort.Strings(found)
	return fmt.Errorf("template: the template calls the method %s, which is not allowed in a plugin", found[0])
}

// The native Execute methods do not run the template of the plugin: they
// run a private copy. A check of the trees of the plugin is not enough: the
// plugin keeps pointers to them, so a FuncMap function (or a goroutine) can
// change a tree after the check, while the template runs, or add a template
// that {{template}} looks up at run time. The copy is made at each call: a
// deep copy of the tree of every associated template (parse.Tree.Copy), in a
// new namespace (Clone) that the plugin cannot reach. The copy is checked,
// then executed, so what runs is what was checked. A template that the
// plugin adds during the execution is not in the copy: {{template}} does not
// find it.
//
// The copy is of the plugin's template, which is never executed itself:
// Parse and Clone of an html/template keep working after Execute (Go
// refuses them), and html/template escapes the copy at each call.

// textCopy returns a private copy of the namespace of t, and its trees.
func textCopy(t *template.Template) (*template.Template, []*parse.Tree, error) {
	c, err := t.Clone()
	if err != nil {
		return nil, nil, err
	}
	// Clone shares the trees: copy them. c is in c.Templates() only when it
	// has a tree.
	c.Tree = c.Tree.Copy()
	trees := []*parse.Tree{c.Tree}
	for _, a := range c.Templates() {
		if a != c {
			a.Tree = a.Tree.Copy()
		}
		trees = append(trees, a.Tree)
	}
	return c, trees, nil
}

// htmlCopy returns a private copy of the namespace of t, and its trees.
// html/template's Clone copies the trees itself.
func htmlCopy(t *htmltemplate.Template) (*htmltemplate.Template, []*parse.Tree, error) {
	c, err := t.Clone()
	if err != nil {
		return nil, nil, err
	}
	trees := []*parse.Tree{c.Tree}
	for _, a := range c.Templates() {
		trees = append(trees, a.Tree)
	}
	return c, trees, nil
}

// textExecute is (*text/template.Template).Execute for the restricted
// machines: it checks and runs a private copy of t (see above).
func textExecute(t *template.Template, w io.Writer, data any) error {
	c, trees, err := textCopy(t)
	if err != nil {
		return err
	}
	if err := templateRefused(trees); err != nil {
		return err
	}
	return c.Execute(w, data)
}

func textExecuteTemplate(t *template.Template, w io.Writer, name string, data any) error {
	c, trees, err := textCopy(t)
	if err != nil {
		return err
	}
	if err := templateRefused(trees); err != nil {
		return err
	}
	return c.ExecuteTemplate(w, name, data)
}

func htmlExecute(t *htmltemplate.Template, w io.Writer, data any) error {
	c, trees, err := htmlCopy(t)
	if err != nil {
		return err
	}
	if err := templateRefused(trees); err != nil {
		return err
	}
	return c.Execute(w, data)
}

func htmlExecuteTemplate(t *htmltemplate.Template, w io.Writer, name string, data any) error {
	c, trees, err := htmlCopy(t)
	if err != nil {
		return err
	}
	if err := templateRefused(trees); err != nil {
		return err
	}
	return c.ExecuteTemplate(w, name, data)
}
