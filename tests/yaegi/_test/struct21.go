package main

type SecretProvider func(user, realm string) string

type BasicAuth struct {
	Realm   string
	Secrets SecretProvider
}

func (a *BasicAuth) CheckAuth() string { return a.Secrets("me", a.Realm) }

func (a *BasicAuth) secretBasic(user, realm string) string { return a.Realm + "-" + user + "-" + realm }

func main() {
	b := &BasicAuth{Realm: "test"}
	b.Secrets = b.secretBasic
	s := b.CheckAuth()
	println(s)
}

// Output:
// test-me-test
