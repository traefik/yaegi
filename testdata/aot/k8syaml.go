// AOT test program: sigs.k8s.io/yaml (YAML through encoding/json tags and
// methods) on interpreted types.
//
//aot:lib sigs.k8s.io/yaml go.yaml.in/yaml/v2
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"sigs.k8s.io/yaml"
)

// Quantity marshals through encoding/json methods.
type Quantity struct{ Milli int64 }

func (q Quantity) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Quote(strconv.FormatInt(q.Milli, 10) + "m")), nil
}

func (q *Quantity) UnmarshalJSON(b []byte) error {
	s, err := strconv.Unquote(string(b))
	if err != nil {
		return err
	}
	q.Milli, err = strconv.ParseInt(strings.TrimSuffix(s, "m"), 10, 64)
	return err
}

type Phase string

type Container struct {
	Name  string            `json:"name"`
	Image string            `json:"image,omitempty"`
	CPU   Quantity          `json:"cpu"`
	Env   map[string]string `json:"env,omitempty"`
	Ports []int32           `json:"ports,omitempty"`
}

type Meta struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels,omitempty"`
}

type Pod struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Meta       `json:"metadata"`
	Containers []Container `json:"containers"`
	Phase      Phase       `json:"phase,omitempty"`
	Replicas   *int        `json:"replicas,omitempty"`
}

func main() {
	if benchMain(benchOp) {
		return
	}
	r := 3
	p := Pod{APIVersion: "v1", Kind: "Pod", Meta: Meta{"web", map[string]string{"app": "web", "tier": "fe"}},
		Containers: []Container{{Name: "nginx", Image: "nginx:1", CPU: Quantity{250}, Ports: []int32{80, 443}}},
		Phase:      "Running", Replicas: &r}
	b, err := yaml.Marshal(p)
	fmt.Print(string(b))
	fmt.Println(err)
	var q Pod
	err = yaml.Unmarshal(b, &q)
	fmt.Println(err, q.Name, q.Labels["tier"], q.Containers[0].CPU.Milli, q.Phase, *q.Replicas, q.Containers[0].Ports)
	j, err := yaml.YAMLToJSON([]byte("a: 1\nb: [x, {c: true}]\n"))
	fmt.Println(string(j), err)
	y, err := yaml.JSONToYAML([]byte(`{"z":1,"a":{"b":[1,2]}}`))
	fmt.Print(string(y))
	fmt.Println(err)
	err = yaml.UnmarshalStrict([]byte("apiVersion: v1\nbogus: 1\n"), &q)
	fmt.Println(err)
	err = yaml.Unmarshal([]byte("containers: [{name: a, cpu: 12x}]"), &q)
	fmt.Println(err)
}

const benchDoc = `apiVersion: v1
kind: Pod
metadata:
  name: web
  labels: {app: web}
containers:
- name: nginx
  image: nginx:1
  cpu: 250m
  ports: [80, 443]
`

// benchOp: unmarshal then marshal a pod.
func benchOp() {
	var p Pod
	if err := yaml.Unmarshal([]byte(benchDoc), &p); err != nil {
		panic(err)
	}
	if _, err := yaml.Marshal(p); err != nil {
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
