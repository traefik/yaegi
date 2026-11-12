// Promoted methods through embedded values and pointers, method shadowing,
// calling the shadowed method explicitly, promoted methods satisfying
// interfaces, and method sets of structs embedding T versus *T.
package main

import "fmt"

type Animal struct {
	Name string
}

func (a Animal) Speak() string    { return a.Name + " makes a sound" }
func (a Animal) Kind() string     { return "animal" }
func (a *Animal) Rename(n string) { a.Name = n }

type Dog struct {
	Animal
	Breed string
}

func (d Dog) Speak() string { return d.Name + " barks (" + d.Animal.Speak() + ")" }

type Puppy struct {
	*Dog
	Age int
}

func (p Puppy) Kind() string { return "puppy of " + p.Dog.Kind() }

type Speaker interface{ Speak() string }
type Renamer interface{ Rename(string) }

type Logger struct{ lines []string }

func (l *Logger) Log(s string) { l.lines = append(l.lines, s) }

type Service struct {
	Logger
	name string
}

type PService struct {
	*Logger
}

func describe(v any) {
	_, s := v.(Speaker)
	_, r := v.(Renamer)
	fmt.Printf("%T speaker=%v renamer=%v\n", v, s, r)
}

func main() {
	d := Dog{Animal{"rex"}, "lab"}
	fmt.Println(d.Speak())
	fmt.Println(d.Animal.Speak())
	fmt.Println(d.Kind())
	d.Rename("max") // (&d.Animal).Rename
	fmt.Println(d.Name, d.Speak())

	p := Puppy{&d, 1}
	fmt.Println(p.Speak(), "|", p.Kind())
	p.Rename("tiny")
	fmt.Println(d.Name)

	var s Speaker = d
	fmt.Println(s.Speak())
	s = p
	fmt.Println(s.Speak())
	s = d.Animal
	fmt.Println(s.Speak())

	describe(d)
	describe(&d)
	describe(p)
	describe(&p)
	describe(d.Animal)
	describe(&d.Animal)

	var svc Service
	svc.Log("a")
	svc.Log("b")
	fmt.Println(svc.lines, len(svc.Logger.lines))
	ps := PService{&Logger{}}
	ps.Log("x")
	fmt.Println(ps.lines)

	// Method value of a promoted method.
	f := d.Speak
	g := p.Kind
	d.Name = "later"
	fmt.Println(f(), "|", g())

	// Method expression with a promoted method.
	fmt.Println(Dog.Kind(d), (*Dog).Speak(&d))
	rn := (*Dog).Rename
	rn(&d, "expr")
	fmt.Println(d.Name)
}
