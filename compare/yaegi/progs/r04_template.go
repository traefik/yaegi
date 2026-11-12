package main

import (
	"os"
	"strings"
	"text/template"
)

type User struct {
	Name  string
	Roles []string
}

func (u User) Greeting(prefix string) string { return prefix + " " + strings.ToUpper(u.Name) }
func (u *User) IsAdmin() bool              { return len(u.Roles) > 0 && u.Roles[0] == "admin" }

func main() {
	t := template.Must(template.New("t").Funcs(template.FuncMap{"join": strings.Join}).Parse(
		`{{.Greeting "hi"}} admin={{.IsAdmin}} roles={{join .Roles ","}} {{range $i, $r := .Roles}}[{{$i}}:{{$r}}]{{end}}` + "\n"))
	if err := t.Execute(os.Stdout, &User{"bob", []string{"admin", "dev"}}); err != nil {
		panic(err)
	}
}
