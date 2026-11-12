package main

import (
	"fmt"

	"github.com/go-playground/validator/v10"
)

type Cfg struct {
	Name  string `validate:"required,min=3"`
	Email string `validate:"email"`
	Port  int    `validate:"gte=1,lte=65535"`
}

func main() {
	v := validator.New()
	fmt.Println(v.Struct(Cfg{Name: "abc", Email: "a@b.io", Port: 80}))
	err := v.Struct(Cfg{Name: "a", Email: "nope", Port: 0})
	for _, e := range err.(validator.ValidationErrors) {
		fmt.Println(e.Field(), e.Tag())
	}
}
