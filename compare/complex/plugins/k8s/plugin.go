package k8splug

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/yaml"
)

// state uses client-go, the biggest dependency tree of this study: a fake
// clientset (the real typed clients over an object tracker), the scheme and
// its codecs, label selectors, unstructured objects and YAML manifests.
type state struct {
	cs kubernetes.Interface
}

const manifest = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: whoami
  namespace: default
  labels: {app: whoami, tier: web}
spec:
  replicas: 3
  selector: {matchLabels: {app: whoami}}
  template:
    metadata: {labels: {app: whoami}}
    spec:
      containers:
      - name: whoami
        image: traefik/whoami:v1.10
        ports: [{containerPort: 80}]
`

func setup(ctx context.Context) (*state, error) {
	obj, gvk, err := scheme.Codecs.UniversalDeserializer().Decode([]byte(manifest), nil, nil)
	if err != nil {
		return nil, err
	}
	dep, ok := obj.(*appsv1.Deployment)
	if !ok {
		return nil, fmt.Errorf("decoded %s as %T", gvk, obj)
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "whoami", Namespace: "default", Labels: map[string]string{"app": "whoami"}},
		Spec: corev1.ServiceSpec{Selector: map[string]string{"app": "whoami"},
			Ports: []corev1.ServicePort{{Port: 80, TargetPort: intstr.FromString("http")}}},
	}
	cs := fake.NewSimpleClientset(dep, svc)
	for i := 0; i < 3; i++ {
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("whoami-%d", i), Namespace: "default",
			Labels: map[string]string{"app": "whoami", "idx": fmt.Sprint(i)}},
			Status: corev1.PodStatus{PodIP: fmt.Sprintf("10.42.0.%d", i+10), Phase: corev1.PodRunning}}
		if _, err := cs.CoreV1().Pods("default").Create(ctx, pod, metav1.CreateOptions{}); err != nil {
			return nil, err
		}
	}
	return &state{cs: cs}, nil
}

func (s *state) process(req *http.Request) (string, error) {
	ctx := req.Context()
	sel := labels.SelectorFromSet(labels.Set{"app": "whoami"})
	pods, err := s.cs.CoreV1().Pods("default").List(ctx, metav1.ListOptions{LabelSelector: sel.String()})
	if err != nil {
		return "", err
	}
	var ips []string
	for _, p := range pods.Items {
		ips = append(ips, p.Status.PodIP)
	}
	name := "req-" + strings.ReplaceAll(strings.Trim(req.URL.Path, "/"), "/", "-")
	if name == "req-" {
		name = "req-root"
	}
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: strings.ReplaceAll(name, ".", "-"), Namespace: "default"},
		Data: map[string]string{"method": req.Method, "host": req.Host}}
	_, err = s.cs.CoreV1().ConfigMaps("default").Create(ctx, cm, metav1.CreateOptions{})
	created := err == nil
	cms, err := s.cs.CoreV1().ConfigMaps("default").List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", err
	}
	dep, err := s.cs.AppsV1().Deployments("default").Get(ctx, "whoami", metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	// Typed to unstructured, and to YAML.
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(dep)
	if err != nil {
		return "", err
	}
	image, _, _ := unstructured.NestedString(u, "spec", "template", "spec", "containers", "0", "image")
	containers, _, _ := unstructured.NestedSlice(u, "spec", "template", "spec", "containers")
	y, err := yaml.Marshal(cm)
	if err != nil {
		return "", err
	}
	_, getErr := s.cs.CoreV1().Secrets("default").Get(ctx, "missing", metav1.GetOptions{})
	return fmt.Sprintf("pods=%v created=%t cms=%d replicas=%d image=%q containers=%d yaml=%d notfound=%v",
		ips, created, len(cms.Items), *dep.Spec.Replicas, image, len(containers), len(y), getErr), nil
}
