package common

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// ServiceDetails -
type ServiceDetails struct {
	Name      string
	Namespace string
	AppLabel  string
	Selector  map[string]string
	Port      int32
}

// Service func
func service(svcInfo *ServiceDetails) *corev1.Service {

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcInfo.Name,
			Namespace: svcInfo.Namespace,
			Labels:    GetLabels(svcInfo.Name, svcInfo.AppLabel),
		},
		Spec: corev1.ServiceSpec{
			Selector: svcInfo.Selector,
			Ports: []corev1.ServicePort{
				{
					Name:     "api",
					Port:     svcInfo.Port,
					Protocol: corev1.ProtocolTCP,
				},
			},
		},
	}
}

// CreateOrUpdateService -
func CreateOrUpdateService(c client.Client, log logr.Logger, svc *corev1.Service, svcInfo *ServiceDetails) (*corev1.Service, controllerutil.OperationResult, error) {

	op, err := controllerutil.CreateOrUpdate(context.TODO(), c, svc, func() error {
		svc.ObjectMeta.Labels = service(svcInfo).ObjectMeta.Labels
		svc.Spec.Selector = service(svcInfo).Spec.Selector
		svc.Spec.Ports = service(svcInfo).Spec.Ports

		return nil
	})

	return svc, op, err

}
