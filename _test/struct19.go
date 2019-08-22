package main

import "fmt"

type Config struct {
	Users        `json:"users,omitempty" mapstructure:","`
	UsersFile    string `json:"usersFile,omitempty"`
	Realm        string `json:"realm,omitempty"`
	RemoveHeader bool   `json:"removeHeader,omitempty"`
	HeaderField  string `json:"headerField,omitempty" export:"true"`
}

// Users holds a list of users
type Users []string

func CreateConfig() *Config {
	return &Config{}
}

func main() {
	c := CreateConfig()
	fmt.Println(c)
}

// Output:
// &{[]   false }
