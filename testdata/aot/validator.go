// AOT test program: github.com/go-playground/validator/v10 on interpreted types.
//
//aot:lib github.com/go-playground/validator/v10
package main

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
)

type Address struct {
	Street string `validate:"required"`
	City   string `validate:"required,alpha"`
	Zip    string `validate:"len=5,numeric"`
}

type Role string

type User struct {
	Name     string            `validate:"required,min=2,max=10"`
	Email    string            `validate:"required,email"`
	Age      int               `validate:"gte=0,lte=130"`
	Role     Role              `validate:"oneof=admin user"`
	Tags     []string          `validate:"dive,required,lowercase"`
	Addr     *Address          `validate:"required"`
	Others   []Address         `validate:"dive"`
	Meta     map[string]string `validate:"dive,keys,alpha,endkeys,required"`
	Code     string            `validate:"even_len"`
	Password string            `validate:"required"`
	Confirm  string            `validate:"eqfield=Password"`
	Website  string            `validate:"omitempty,url"`
	When     time.Time         `validate:"required"`
}

func evenLen(fl validator.FieldLevel) bool { return len(fl.Field().String())%2 == 0 }

func main() {
	if benchMain(benchOp) {
		return
	}
	v := validator.New(validator.WithRequiredStructEnabled())
	if err := v.RegisterValidation("even_len", evenLen); err != nil {
		fmt.Println(err)
	}
	v.RegisterStructValidation(func(sl validator.StructLevel) {
		u := sl.Current().Interface().(User)
		if u.Role == "admin" && u.Age < 18 {
			sl.ReportError(u.Age, "Age", "Age", "adultadmin", "")
		}
	}, User{})
	good := User{Name: "ann", Email: "ann@example.com", Age: 30, Role: "user", Tags: []string{"a", "b"},
		Addr: &Address{"1 rue", "Lyon", "69000"}, Meta: map[string]string{"k": "v"}, Code: "ab",
		Password: "x", Confirm: "x", When: time.Unix(1, 0)}
	fmt.Println("good:", v.Struct(good))
	bad := User{Name: "a", Email: "nope", Age: 200, Role: "admin", Tags: []string{"OK", ""},
		Addr: &Address{"", "L1", "123"}, Others: []Address{{"s", "c", "12345"}, {}}, Meta: map[string]string{"1": ""},
		Code: "abc", Password: "x", Confirm: "y", Website: "::bad"}
	err := v.Struct(bad)
	var ve validator.ValidationErrors
	fmt.Println(errors.As(err, &ve), len(ve))
	var lines []string
	for _, fe := range ve {
		lines = append(lines, fmt.Sprintf("%s %s %s %v", fe.Namespace(), fe.Tag(), fe.Param(), fe.Value()))
	}
	sort.Strings(lines)
	fmt.Println(strings.Join(lines, "\n"))
	fmt.Println(v.Var("x@y.z", "email"), v.Var(5, "gt=10") != nil, v.Var([]int{1}, "min=2") != nil)
	err = v.Struct(42)
	var ie *validator.InvalidValidationError
	fmt.Println(errors.As(err, &ie), err)
	fmt.Println(v.StructPartial(bad, "Name", "Age") != nil, v.StructExcept(good, "When"))
}

var benchV = validator.New()

// benchOp: validate a valid interpreted struct.
func benchOp() {
	u := User{Name: "ann", Email: "ann@example.com", Age: 30, Role: "user", Tags: []string{"a"},
		Addr: &Address{"1 rue", "Lyon", "69000"}, Code: "ab", Password: "x", Confirm: "x", When: time.Unix(1, 0)}
	benchV.RegisterValidation("even_len", evenLen)
	if err := benchV.Struct(u); err != nil {
		panic(err)
	}
}

func benchMain(op func()) bool {
	n, _ := strconv.Atoi(os.Getenv("AOT_BENCH"))
	if n <= 0 {
		return false
	}
	op()
	t0 := time.Now()
	for i := 0; i < n; i++ {
		op()
	}
	fmt.Printf("BENCH %d\n", time.Since(t0).Nanoseconds()/int64(n))
	return true
}
