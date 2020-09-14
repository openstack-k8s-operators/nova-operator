/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/prometheus/common/log"

	routev1 "github.com/openshift/api/route/v1"

	novav1beta1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/nova-operator/controllers"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(novav1beta1.AddToScheme(scheme))
	utilruntime.Must(routev1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	namespace, err := getWatchNamespace()
	if err != nil {
		setupLog.Error(err, "failed to get WatchNamespace")
		os.Exit(1)

	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		Port:               9443,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "f33036c1.openstack.org",
		Namespace:          namespace,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}
	kclient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	if err = (&controllers.IscsidReconciler{
		Client:  mgr.GetClient(),
		Kclient: kclient,
		Log:     ctrl.Log.WithName("controllers").WithName("Iscsid"),
		Scheme:  mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Iscsid")
		os.Exit(1)
	}
	if err = (&controllers.VirtlogdReconciler{
		Client:  mgr.GetClient(),
		Kclient: kclient,
		Log:     ctrl.Log.WithName("controllers").WithName("Virtlogd"),
		Scheme:  mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Virtlogd")
		os.Exit(1)
	}
	if err = (&controllers.LibvirtdReconciler{
		Client:  mgr.GetClient(),
		Kclient: kclient,
		Log:     ctrl.Log.WithName("controllers").WithName("Libvirtd"),
		Scheme:  mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Libvirtd")
		os.Exit(1)
	}
	if err = (&controllers.NovaMigrationTargetReconciler{
		Client:  mgr.GetClient(),
		Kclient: kclient,
		Log:     ctrl.Log.WithName("controllers").WithName("NovaMigrationTarget"),
		Scheme:  mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NovaMigrationTarget")
		os.Exit(1)
	}
	if err = (&controllers.NovaComputeReconciler{
		Client:  mgr.GetClient(),
		Kclient: kclient,
		Log:     ctrl.Log.WithName("controllers").WithName("NovaCompute"),
		Scheme:  mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NovaCompute")
		os.Exit(1)
	}
	if err = (&controllers.NovaAPIReconciler{
		Client:  mgr.GetClient(),
		Kclient: kclient,
		Log:     ctrl.Log.WithName("controllers").WithName("NovaAPI"),
		Scheme:  mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NovaAPI")
		os.Exit(1)
	}
	if err = (&controllers.NovaSchedulerReconciler{
		Client:  mgr.GetClient(),
		Kclient: kclient,
		Log:     ctrl.Log.WithName("controllers").WithName("NovaScheduler"),
		Scheme:  mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NovaScheduler")
		os.Exit(1)
	}
	if err = (&controllers.NovaReconciler{
		Client:  mgr.GetClient(),
		Kclient: kclient,
		Log:     ctrl.Log.WithName("controllers").WithName("Nova"),
		Scheme:  mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Nova")
		os.Exit(1)
	}
	if err = (&controllers.NovaConductorReconciler{
		Client:  mgr.GetClient(),
		Kclient: kclient,
		Log:     ctrl.Log.WithName("controllers").WithName("NovaConductor"),
		Scheme:  mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NovaConductor")
		os.Exit(1)
	}
	if err = (&controllers.NovaCellReconciler{
		Client:  mgr.GetClient(),
		Kclient: kclient,
		Log:     ctrl.Log.WithName("controllers").WithName("NovaCell"),
		Scheme:  mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NovaCell")
		os.Exit(1)
	}
	if err = (&controllers.NovaMetadataReconciler{
		Client:  mgr.GetClient(),
		Kclient: kclient,
		Log:     ctrl.Log.WithName("controllers").WithName("NovaMetadata"),
		Scheme:  mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NovaMetadata")
		os.Exit(1)
	}
	if err = (&controllers.NovaNoVNCProxyReconciler{
		Client:  mgr.GetClient(),
		Kclient: kclient,
		Log:     ctrl.Log.WithName("controllers").WithName("NovaNoVNCProxy"),
		Scheme:  mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NovaNoVNCProxy")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

// getWatchNamespace returns the Namespace the operator should be watching for changes
func getWatchNamespace() (string, error) {
	// WatchNamespaceEnvVar is the constant for env variable WATCH_NAMESPACE
	// which specifies the Namespace to watch.
	// An empty value means the operator is running with cluster scope.
	var watchNamespaceEnvVar = "WATCH_NAMESPACE"

	ns, found := os.LookupEnv(watchNamespaceEnvVar)
	if !found {
		return "", fmt.Errorf("%s must be set", watchNamespaceEnvVar)
	}
	return ns, nil
}
