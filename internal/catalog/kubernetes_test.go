package catalog

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestSourceFromAnnotatedKubernetesService(t *testing.T) {
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "identity-service", Namespace: "platform", Annotations: map[string]string{
		annotationPrefix + "enabled": "true", annotationPrefix + "title": "Identity API", annotationPrefix + "port": "http", annotationPrefix + "path": "/swagger/doc.json",
	}}, Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "http", Port: 8080, TargetPort: intstr.FromInt32(8080)}}}}
	source, ok := sourceFromService(service)
	if !ok {
		t.Fatal("expected annotated Service to be discovered")
	}
	if source.Name != "platform--identity-service" || source.Title != "Identity API" || source.URL != "http://identity-service.platform.svc:8080/swagger/doc.json" {
		t.Fatalf("unexpected source: %+v", source)
	}
}

func TestSourceFromServiceRequiresOptInAndValidPort(t *testing.T) {
	for name, service := range map[string]*corev1.Service{
		"not opted in": {ObjectMeta: metav1.ObjectMeta{Name: "users", Namespace: "platform"}, Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "http", Port: 8080}}}},
		"missing port": {ObjectMeta: metav1.ObjectMeta{Name: "users", Namespace: "platform", Annotations: map[string]string{annotationPrefix + "enabled": "true", annotationPrefix + "port": "docs"}}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, ok := sourceFromService(service); ok {
				t.Fatal("expected Service to be ignored")
			}
		})
	}
}
