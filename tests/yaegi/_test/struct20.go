package main

type SecretProvider func(user, realm string) string

type BasicAuth struct {
	Realm   string
	Secrets SecretProvider
}

func (a *BasicAuth) CheckAuth() string { return a.Secrets("me", a.Realm) }

func secretBasic(user, realm string) string { return user + "-" + realm }

func main() {
	b := &BasicAuth{"test", secretBasic}
	s := b.CheckAuth()
	println(s)
}

// Output:
// me-test
