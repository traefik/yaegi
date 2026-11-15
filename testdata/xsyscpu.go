// A program run from source that imports golang.org/x/sys/cpu, here through
// github.com/go-playground/validator/v10 (validator imports
// golang.org/x/crypto/sha3, which imports x/sys/cpu). The init of x/sys/cpu
// calls functions written in assembly (cpuid and xgetbv on x86, getisar0...
// on arm64, stfle on s390x). Before, the VM refused the program: "cpuid has
// no Go body". This is what a plugin got when it vendored another version of
// validator than the AOT copy, or when the host was a newer Go patch release
// than the AOT copies. Now these functions return zeros in the VM: a CPU
// without extensions, so the libraries take their pure Go code. The program
// prints nothing that depends on the CPU features, so its output is the one
// of go run.
package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/go-playground/validator/v10"
	"golang.org/x/sys/cpu"
)

type Address struct {
	City string `validate:"required,alpha"`
	Zip  string `validate:"len=5,numeric"`
}

type User struct {
	Name  string   `validate:"required,min=2,max=10"`
	Email string   `validate:"required,email"`
	Age   int      `validate:"gte=0,lte=130"`
	Tags  []string `validate:"dive,required,lowercase"`
	Addr  *Address `validate:"required"`
}

func main() {
	fmt.Println("cpu initialized:", cpu.Initialized)

	v := validator.New(validator.WithRequiredStructEnabled())
	good := User{Name: "ann", Email: "ann@example.com", Age: 30, Tags: []string{"a"}, Addr: &Address{"Lyon", "69000"}}
	fmt.Println("good:", v.Struct(good))

	bad := User{Name: "a", Email: "nope", Age: 200, Tags: []string{"OK", ""}, Addr: &Address{"L1", "123"}}
	var ve validator.ValidationErrors
	fmt.Println(errors.As(v.Struct(bad), &ve), len(ve))
	var lines []string
	for _, fe := range ve {
		lines = append(lines, fmt.Sprintf("%s %s %s %v", fe.Namespace(), fe.Tag(), fe.Param(), fe.Value()))
	}
	sort.Strings(lines)
	fmt.Println(strings.Join(lines, "\n"))

	fmt.Println(v.Var("x@y.z", "email"), v.Var(5, "gt=10") != nil)
}
