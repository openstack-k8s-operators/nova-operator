package novacell

import (
	"path/filepath"

	"github.com/go-logr/logr"

	novav1beta1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	common "github.com/openstack-k8s-operators/nova-operator/pkg/common"

	util "github.com/openstack-k8s-operators/lib-common/pkg/util"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type novaConfigOptions struct {
	KeystoneEndpoint string
}

// ScriptsConfigMap - scripts config map
func ScriptsConfigMap(cr *novav1beta1.NovaCell, scheme *runtime.Scheme, log logr.Logger) *corev1.ConfigMap {
	opts := novaConfigOptions{"FIXME"}

	// get templates base path, either running local or deployed as container
	templatesPath := util.GetTemplatesPath()

	// get all scripts templates which are in ../templesPath/api.Kind/bin
	templatesFiles := util.GetAllTemplates(templatesPath, cr.Kind, "bin")

	data := make(map[string]string)
	// render all template files
	for _, file := range templatesFiles {
		data[filepath.Base(file)] = util.ExecuteTemplate(file, opts)
	}

	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-scripts",
			Namespace: cr.Namespace,
			Labels:    common.GetLabels(cr.Name, AppLabel),
		},
		Data: data,
	}
	cm.ObjectMeta.Labels["upper-cr"] = cr.Name

	controllerutil.SetControllerReference(cr, cm, scheme)

	return cm
}

// ConfigMap - config map containing mandatory auto rendered config files for the service
func ConfigMap(cr *novav1beta1.NovaCell, scheme *runtime.Scheme) *corev1.ConfigMap {
	opts := novaConfigOptions{"FIXME"}

	// get templates base path, either running local or deployed as container
	templatesPath := util.GetTemplatesPath()

	// get all scripts templates which are in ../templesPath/api.Kind/config
	templatesFiles := util.GetAllTemplates(templatesPath, cr.Kind, "config")

	data := make(map[string]string)
	// render all template files
	for _, file := range templatesFiles {
		data[filepath.Base(file)] = util.ExecuteTemplate(file, opts)
	}

	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-config-data",
			Namespace: cr.Namespace,
			Labels:    common.GetLabels(cr.Name, AppLabel),
		},
		Data: data,
	}
	cm.ObjectMeta.Labels["upper-cr"] = cr.Name

	controllerutil.SetControllerReference(cr, cm, scheme)

	return cm
}

// CustomConfigMap - config map used by the user to customize the service
func CustomConfigMap(cr *novav1beta1.NovaCell, scheme *runtime.Scheme) *corev1.ConfigMap {

	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-config-data-custom",
			Namespace: cr.Namespace,
			Labels:    common.GetLabels(cr.Name, AppLabel),
		},
		Data: map[string]string{},
	}
	cm.ObjectMeta.Labels["upper-cr"] = cr.Name

	controllerutil.SetControllerReference(cr, cm, scheme)

	return cm
}
