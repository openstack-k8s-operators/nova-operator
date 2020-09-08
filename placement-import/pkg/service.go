package placement

import (
	placementv1beta1 "github.com/openstack-k8s-operators/placement-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// Service func
func Service(api *placementv1beta1.PlacementAPI, scheme *runtime.Scheme) *corev1.Service {

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      api.Name,
			Namespace: api.Namespace,
			Labels:    GetLabels(api.Name),
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": AppLabel},
			Ports: []corev1.ServicePort{
				{Name: "api", Port: 8778, Protocol: corev1.ProtocolTCP},
			},
		},
	}
	controllerutil.SetControllerReference(api, svc, scheme)
	return svc
}
