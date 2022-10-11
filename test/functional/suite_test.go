/*
Copyright 2022.

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

package functional_test

import (
	"context"
	"fmt"
	"go/build"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/mod/modfile"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"
	novav1beta1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/nova-operator/controllers"
	nova_common "github.com/openstack-k8s-operators/nova-operator/pkg/common"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
)

func GetDependencyVersion(moduleName string) (string, error) {
	content, err := os.ReadFile("../../go.mod")
	if err != nil {
		return "", err
	}

	f, err := modfile.Parse("go.mod", content, nil)
	if err != nil {
		return "", err
	}

	for _, r := range f.Require {
		if r.Mod.Path == moduleName {
			return r.Mod.Version, nil
		}
	}
	return "", fmt.Errorf("Cannot find %s in our go.mod file", moduleName)

}

func GetCRDDirFromModule(moduleName string) string {
	version, err := GetDependencyVersion(moduleName)
	Expect(err).NotTo(HaveOccurred())
	versionedModule := fmt.Sprintf("%s@%s", moduleName, version)
	path := filepath.Join(build.Default.GOPATH, "pkg", "mod", versionedModule, "bases")
	return path
}

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "config", "crd", "bases"),
			// NOTE(gibi): we need to list all the external CRDs our operator depends on
			GetCRDDirFromModule("github.com/openstack-k8s-operators/mariadb-operator/api"),
		},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	// NOTE(gibi): Need to add all API schemas our operator can own.
	// Keep this in synch with NovaAPIReconciler.SetupWithManager,
	// otherwise the reconciler loop will silently not start
	// in the test env.
	err = novav1beta1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = mariadbv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// Start the controller-manager in a goroutine
	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	Expect(err).ToNot(HaveOccurred())

	kclient, err := kubernetes.NewForConfig(cfg)
	Expect(err).ToNot(HaveOccurred(), "failed to create kclient")

	err = (&controllers.NovaAPIReconciler{
		ReconcilerBase: nova_common.ReconcilerBase{
			Client:  k8sManager.GetClient(),
			Scheme:  k8sManager.GetScheme(),
			Kclient: kclient,
			Log:     ctrl.Log.WithName("controllers").WithName("NovaApi"),
		},
		RequeueTimeoutSeconds: 1,
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&controllers.NovaReconciler{
		ReconcilerBase: nova_common.ReconcilerBase{
			Client:  k8sManager.GetClient(),
			Scheme:  k8sManager.GetScheme(),
			Kclient: kclient,
			Log:     ctrl.Log.WithName("controllers").WithName("Nova"),
		},
		RequeueTimeout: time.Duration(100) * time.Millisecond,
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&controllers.NovaConductorReconciler{
		ReconcilerBase: nova_common.ReconcilerBase{
			Client:  k8sManager.GetClient(),
			Scheme:  k8sManager.GetScheme(),
			Kclient: kclient,
			Log:     ctrl.Log.WithName("controllers").WithName("NovaConductor"),
		},
		RequeueTimeoutSeconds: 1,
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&controllers.NovaCellReconciler{
		ReconcilerBase: nova_common.ReconcilerBase{
			Client:  k8sManager.GetClient(),
			Scheme:  k8sManager.GetScheme(),
			Kclient: kclient,
			Log:     ctrl.Log.WithName("controllers").WithName("NovaApi"),
		},
	}).SetupWithManager(k8sManager)

	Expect(err).ToNot(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()

})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
