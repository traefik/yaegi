// github.com/go-playground/validator/v10: tags on interpreted structs,
// nested and dive, custom validation funcs, struct-level validation,
// error messages and field names.
package main

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

type Address struct {
	Street string `validate:"required"`
	City   string `validate:"required,min=2"`
}

type User struct {
	Name     string            `json:"name" validate:"required,min=3,max=10"`
	Email    string            `json:"email" validate:"required,email"`
	Age      int               `json:"age" validate:"gte=0,lte=130"`
	Role     string            `json:"role" validate:"oneof=admin user"`
	Addrs    []Address         `json:"addrs" validate:"required,dive"`
	Tags     map[string]string `json:"tags" validate:"dive,keys,alpha,endkeys,required"`
	Code     string            `json:"code" validate:"is-even-len"`
	Password string            `json:"password"`
	Confirm  string            `json:"confirm" validate:"eqfield=Password"`
	Ptr      *Address          `json:"ptr" validate:"omitempty"`
}

type Level int

func main() {
	v := validator.New(validator.WithRequiredStructEnabled())
	v.RegisterTagNameFunc(func(f reflect.StructField) string {
		return strings.SplitN(f.Tag.Get("json"), ",", 2)[0]
	})
	err := v.RegisterValidation("is-even-len", func(fl validator.FieldLevel) bool {
		return len(fl.Field().String())%2 == 0
	})
	fmt.Println(err)
	v.RegisterStructValidation(func(sl validator.StructLevel) {
		u := sl.Current().Interface().(User)
		if u.Name == "root" {
			sl.ReportError(u.Name, "name", "Name", "notroot", "")
		}
	}, User{})

	good := User{Name: "alice", Email: "a@b.co", Age: 30, Role: "admin", Addrs: []Address{{"s", "Lyon"}}, Tags: map[string]string{"k": "v"}, Code: "ab", Password: "p", Confirm: "p"}
	fmt.Println(v.Struct(good))

	bad := User{Name: "al", Email: "nope", Age: 200, Role: "x", Addrs: []Address{{"", "L"}}, Tags: map[string]string{"k1": ""}, Code: "abc", Password: "p", Confirm: "q", Ptr: &Address{}}
	err = v.Struct(bad)
	var ve validator.ValidationErrors
	fmt.Println(errors.As(err, &ve))
	for _, fe := range ve {
		fmt.Println(fe.Namespace(), fe.Field(), fe.StructField(), fe.Tag(), fe.Param(), fe.Kind(), fe.Value())
	}
	fmt.Println(v.Struct(User{Name: "root", Email: "r@r.r", Role: "user", Addrs: []Address{{"a", "bb"}}}))
	fmt.Println(v.Var("x@y.z", "email"), v.Var(Level(5), "max=3"))
	fmt.Println(v.Struct(42))
}
