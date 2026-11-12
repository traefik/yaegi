package main

import (
	"fmt"

	"github.com/tidwall/gjson"
)

const doc = `{"name":{"first":"Tom","last":"Anderson"},"age":37,"children":["Sara","Alex","Jack"],"friends":[{"first":"Dale","age":44},{"first":"Roger","age":68}]}`

func main() {
	fmt.Println(gjson.Get(doc, "name.last").String())
	fmt.Println(gjson.Get(doc, "age").Int())
	fmt.Println(gjson.Get(doc, "children.#").Int(), gjson.Get(doc, "children.1"))
	fmt.Println(gjson.Get(doc, `friends.#(age>45).first`))
	gjson.Get(doc, "friends").ForEach(func(k, v gjson.Result) bool {
		fmt.Println(v.Get("first"), v.Get("age").Int())
		return true
	})
	fmt.Println(gjson.GetBytes([]byte(doc), "name.first").String(), gjson.Valid(doc))
}
