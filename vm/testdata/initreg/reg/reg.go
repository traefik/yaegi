// Package reg is a registry that packages a and b fill in their init
// (testdata/initregistry.go).
package reg

var names []string

// Add registers name.
func Add(name string) { names = append(names, name) }

// Names returns the registered names, in the order of the calls to Add.
func Names() []string { return names }

func init() { println("init reg") }
