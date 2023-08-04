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

package controllers

import (
	"context"
	"fmt"
	"os"
	"path"

	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	routev1 "github.com/openshift/api/route/v1"
	common "github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	util "github.com/openstack-k8s-operators/lib-common/modules/common/util"
	"github.com/openstack-k8s-operators/lib-common/modules/storage"
	aee "github.com/openstack-k8s-operators/openstack-ansibleee-operator/api/v1alpha1"

	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
)

// NovaExternalComputeReconciler reconciles a NovaExternalCompute object
type NovaExternalComputeReconciler struct {
	ReconcilerBase
}

// +kubebuilder:rbac:groups=nova.openstack.org,resources=novaexternalcomputes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nova.openstack.org,resources=novaexternalcomputes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=nova.openstack.org,resources=novaexternalcomputes/finalizers,verbs=update
// +kubebuilder:rbac:groups=ansibleee.openstack.org,resources=openstackansibleees,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete;

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *NovaExternalComputeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, _err error) {
	l := log.FromContext(ctx)

	// Fetch the instance that needs to be reconciled
	instance := &novav1.NovaExternalCompute{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected.
			// For additional cleanup logic use finalizers. Return and don't requeue.
			l.Info("NovaExternalCompute instance not found, probably deleted before reconciled. Nothing to do.")
			return ctrl.Result{}, nil
		}
		l.Error(err, "Failed to read the NovaExternalCompute instance.")
		return ctrl.Result{}, err
	}

	h, err := helper.NewHelper(
		instance,
		r.Client,
		r.Kclient,
		r.Scheme,
		r.Log,
	)

	if err != nil {
		l.Error(err, "Failed to create lib-common Helper")
		return ctrl.Result{}, err
	}
	l.Info("Reconciling")

	// initialize status fields
	if err = r.initStatus(ctx, h, instance); err != nil {
		return ctrl.Result{}, err
	}

	// Always update the instance status when exiting this function so we can
	// persist any changes happened during the current reconciliation.
	defer func() {
		// update the overall status condition if service is ready
		if allSubConditionIsTrue(instance.Status) {
			instance.Status.Conditions.MarkTrue(
				condition.ReadyCondition, condition.ReadyMessage)
		} else {
			// something is not ready so reset the Ready condition
			instance.Status.Conditions.MarkUnknown(
				condition.ReadyCondition, condition.InitReason, condition.ReadyInitMessage)
			// and recalculate it based on the state of the rest of the conditions
			instance.Status.Conditions.Set(
				instance.Status.Conditions.Mirror(condition.ReadyCondition))
		}
		err := h.PatchInstance(ctx, instance)
		if err != nil {
			_err = err
			return
		}
	}()

	if err = r.ensurePlaybooks(ctx, h, instance); err != nil {
		return ctrl.Result{}, err
	}

	if !instance.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.reconcileDelete(ctx, h, instance)
	}

	updated := controllerutil.AddFinalizer(instance, h.GetFinalizer())
	if updated {
		l.Info("Added finalizer to ourselves")
		// we intentionally return immediately to force the deferred function
		// to persist the Instance with the finalizer. We need to have our own
		// finalizer persisted before we deploy any compute to avoid orphaning
		// the compute rows in our database during CR deletion.
		return ctrl.Result{}, nil
	}

	// TODO(gibi): Can we use a simple map[string][string] for hashes?
	// Collect hashes of all the input we depend on so that we can easily
	// detect if something is changed.
	hashes := make(map[string]env.Setter)

	inventoryHash, result, err := ensureConfigMap(
		ctx,

		types.NamespacedName{Namespace: instance.Namespace, Name: instance.Spec.InventoryConfigMapName},
		// NOTE(gibi): Add the fields here we expect to exists in the InventorySecret
		[]string{
			"inventory",
		},
		h.GetClient(),
		&instance.Status.Conditions,
		r.RequeueTimeout,
	)
	if err != nil {
		return result, err
	}
	hashes[instance.Spec.InventoryConfigMapName] = env.SetValue(inventoryHash)

	sshKeyHHash, result, _, err := ensureSecret(
		ctx,
		types.NamespacedName{Namespace: instance.Namespace, Name: instance.Spec.SSHKeySecretName},
		// NOTE(gibi): Add the fields here we expect to exists in the SSHKeySecret
		// This is based on the structure defined in the schema of kubernetes.io/ssh-auth
		//  https://kubernetes.io/docs/concepts/configuration/secret/#ssh-authentication-secrets
		[]string{
			"ssh-privatekey",
		},
		h.GetClient(),
		&instance.Status.Conditions,
		r.RequeueTimeout,
	)
	if err != nil {
		return result, err
	}
	hashes[instance.Spec.SSHKeySecretName] = env.SetValue(sshKeyHHash)

	cell, result, err := r.ensureCellReady(ctx, instance)
	if (err != nil || result != ctrl.Result{}) {
		return result, err
	}

	instanceSecretHash, result, secret, err := ensureSecret(
		ctx,
		types.NamespacedName{Namespace: instance.Namespace, Name: cell.Spec.Secret},
		// NOTE(sean): The nova external compute uses the cell secret to lookup the
		// nova keystone user password. Add any additional fields here we expect to
		// exists in the cell secret if we start using additional values in the future.
		[]string{
			ServicePasswordSelector,
		},
		h.GetClient(),
		&instance.Status.Conditions,
		r.RequeueTimeout,
	)
	if err != nil {
		return result, err
	}
	hashes[cell.Spec.Secret] = env.SetValue(instanceSecretHash)

	// TODO(gibi): gather the information from the cell we need for the config
	// and hash that into our input hash

	// all our input checks out so report InputReady
	instance.Status.Conditions.MarkTrue(condition.InputReadyCondition, condition.InputReadyMessage)

	// TODO(gibi): generate service config here and include the hash of that
	// into the hashes

	err = r.ensureConfigMaps(ctx, h, instance, cell, &hashes, secret)
	if err != nil {
		return ctrl.Result{}, err
	}

	// create hash over all the different input resources to identify if any of
	// those changed and a restart/recreate is required.
	inputHash, err := util.HashOfInputHashes(hashes)
	if err != nil {
		return ctrl.Result{}, err
	}
	instance.Status.Hash[common.InputHashName] = inputHash

	// we support stopping before deploying the compute node
	// so only create the AAE CRs if we have deployment enabled.
	if *instance.Spec.Deploy {
		l.Info("Deploying libvirt and nova")
		// create all AEE resource in parallel
		libvirtAEE, err := r.ensureAEEDeployLibvirt(ctx, h, instance)
		if err != nil {
			return ctrl.Result{}, err
		}
		novaAEE, err := r.ensureAEEDeployNova(ctx, h, instance)
		if err != nil {
			return ctrl.Result{}, err
		}

		// then check if they have completed and requeue if not.
		if libvirtAEE.Status.JobStatus != "Succeeded" {
			return ctrl.Result{RequeueAfter: r.RequeueTimeout}, fmt.Errorf(
				"libvirt deployment %s for %s", libvirtAEE.Status.JobStatus, instance.Name,
			)
		}
		if novaAEE.Status.JobStatus != "Succeeded" {
			return ctrl.Result{RequeueAfter: r.RequeueTimeout}, fmt.Errorf(
				"nova deployment %s for %s", novaAEE.Status.JobStatus, instance.Name,
			)
		}

		// we only get here if we completed successfully so we can just delete themg
		err = r.cleanupAEE(ctx, h, libvirtAEE)
		if err != nil {
			return ctrl.Result{}, err
		}
		err = r.cleanupAEE(ctx, h, novaAEE)
		if err != nil {
			return ctrl.Result{}, err
		}
		// only mark true if we have completed all steps
		// this should be at the end of the function to ensure we don't set the status
		// if we can fail later in the function.
		instance.Status.Conditions.MarkTrue(
			condition.DeploymentReadyCondition, condition.DeploymentReadyMessage,
		)
	}
	l.Info("Reconcile complete")
	return ctrl.Result{}, nil
}

func (r *NovaExternalComputeReconciler) initStatus(
	ctx context.Context, h *helper.Helper, instance *novav1.NovaExternalCompute,
) error {
	if err := r.initConditions(ctx, h, instance); err != nil {
		return err
	}

	if instance.Status.Hash == nil {
		instance.Status.Hash = map[string]string{}
	}

	return nil
}

func (r *NovaExternalComputeReconciler) initConditions(
	ctx context.Context, h *helper.Helper, instance *novav1.NovaExternalCompute,
) error {
	if instance.Status.Conditions == nil {
		instance.Status.Conditions = condition.Conditions{}
		// initialize all conditions to Unknown
		cl := condition.CreateList(
			// TODO(gibi): Initialize each condition the controller reports
			// here to Unknown. By default only the top level Ready condition is
			// created by Conditions.Init()
			condition.UnknownCondition(
				condition.InputReadyCondition,
				condition.InitReason,
				condition.InputReadyInitMessage,
			),
			condition.UnknownCondition(
				novav1.NovaCellReadyCondition,
				condition.InitReason,
				novav1.NovaCellReadyInitMessage,
				instance.Spec.CellName,
			),
			condition.UnknownCondition(
				condition.ServiceConfigReadyCondition,
				condition.InitReason,
				condition.ServiceConfigReadyInitMessage,
			),
			condition.UnknownCondition(
				condition.DeploymentReadyCondition,
				condition.InitReason,
				condition.DeploymentReadyInitMessage,
			),
		)
		instance.Status.Conditions.Init(&cl)
	}
	return nil
}

func (r *NovaExternalComputeReconciler) reconcileDelete(
	ctx context.Context,
	h *helper.Helper,
	instance *novav1.NovaExternalCompute,
) error {
	l := log.FromContext(ctx)
	l.Info("Reconciling delete")

	// TODO(gibi): A compute is being removed from the system so we might want
	// to clean up the compute Service and the ComputeNode from the cell
	// database

	// Successfully cleaned up everything. So as the final step let's remove the
	// finalizer from ourselves to allow the deletion of the CR itself
	updated := controllerutil.RemoveFinalizer(instance, h.GetFinalizer())
	if updated {
		l.Info("Removed finalizer from ourselves")
	}

	l.Info("Reconciled delete successfully")
	return nil
}

func (r *NovaExternalComputeReconciler) ensureCellReady(
	ctx context.Context,
	instance *novav1.NovaExternalCompute,
) (*novav1.NovaCell, ctrl.Result, error) {
	cell := &novav1.NovaCell{}
	cellCRName := getNovaCellCRName(instance.Spec.NovaInstance, instance.Spec.CellName)
	err := r.Client.Get(
		ctx, types.NamespacedName{
			Namespace: instance.Namespace,
			Name:      cellCRName,
		},
		cell,
	)

	if err != nil && !k8s_errors.IsNotFound(err) {
		instance.Status.Conditions.Set(
			condition.FalseCondition(
				novav1.NovaCellReadyCondition,
				condition.ErrorReason,
				condition.SeverityError,
				novav1.NovaCellReadyErrorMessage,
				cellCRName,
				err.Error(),
			),
		)
		return nil, ctrl.Result{}, fmt.Errorf("failed to query NovaCells %w", err)
	}

	if k8s_errors.IsNotFound(err) {
		instance.Status.Conditions.Set(
			condition.FalseCondition(
				novav1.NovaCellReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				novav1.NovaCellReadyNotExistsMessage,
				cellCRName,
			),
		)
		// Here we need to wait for the NovaCell to be created so we requeue explicitly
		return nil, ctrl.Result{RequeueAfter: r.RequeueTimeout}, nil
	}

	// We cannot move forward while the cell is not ready as we need to gather
	// information from the NovaCell to generate the compute config
	if !cell.IsReady() {
		instance.Status.Conditions.Set(
			condition.FalseCondition(
				novav1.NovaCellReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				novav1.NovaCellReadyNotReadyMessage,
				cellCRName,
			),
		)
		// TODO(gibi): this should not be an explicit requeue, instead we need
		// to add a watch for the NovaCell and just return without requeue here.
		return cell, ctrl.Result{RequeueAfter: r.RequeueTimeout}, nil
	}

	// our NovaCell is Ready
	instance.Status.Conditions.MarkTrue(
		novav1.NovaCellReadyCondition, novav1.NovaCellReadyMessage, cellCRName)

	return cell, ctrl.Result{}, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *NovaExternalComputeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&novav1.NovaExternalCompute{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&aee.OpenStackAnsibleEE{}).
		Complete(r)
}

func (r *NovaExternalComputeReconciler) ensureConfigMaps(
	ctx context.Context, h *helper.Helper, instance *novav1.NovaExternalCompute,
	cell *novav1.NovaCell, hashes *map[string]env.Setter, secret corev1.Secret,
) error {
	err := r.generateConfigs(ctx, h, instance, cell, hashes, secret)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ServiceConfigReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceConfigReadyErrorMessage,
			err.Error()))
		return err
	}
	instance.Status.Conditions.MarkTrue(
		condition.ServiceConfigReadyCondition, condition.ServiceConfigReadyMessage,
	)
	return nil
}

func (r *NovaExternalComputeReconciler) generateConfigs(
	ctx context.Context, h *helper.Helper, instance *novav1.NovaExternalCompute,
	cell *novav1.NovaCell, hashes *map[string]env.Setter, secret corev1.Secret,
) error {

	cellMessageBusSecret := &corev1.Secret{}
	secretName := types.NamespacedName{
		Namespace: cell.Namespace,
		Name:      cell.Spec.CellMessageBusSecretName,
	}
	err := h.GetClient().Get(ctx, secretName, cellMessageBusSecret)
	if err != nil {
		util.LogForObject(
			h, "Failed reading Secret", instance,
			"CellMessageBusSecretName", cell.Spec.CellMessageBusSecretName)
		return err
	}

	// we should restructure this to be in the same order as the reset of the resources
	// <nova_instance>-<cell_name>-<service_name>-<service_type>
	vncRouteName := fmt.Sprintf("nova-novncproxy-%s-public", cell.Spec.CellName)

	vncRoute := &routev1.Route{}
	err = h.GetClient().Get(ctx, types.NamespacedName{
		Namespace: instance.Namespace,
		Name:      vncRouteName,
	}, vncRoute)
	if err != nil {
		return err
	}
	vncHost := vncRoute.Spec.Host
	if vncHost == "" && len(vncRoute.Status.Ingress) > 0 {
		vncHost = vncRoute.Status.Ingress[0].Host
	} else if vncHost == "" {
		return fmt.Errorf("vncHost is empty")
	}

	templateParameters := map[string]interface{}{
		"service_name":           "nova-external-compute",
		"keystone_internal_url":  cell.Spec.KeystoneAuthURL,
		"nova_keystone_user":     cell.Spec.ServiceUser,
		"compute_driver":         "libvirt.LibvirtDriver",
		"nova_keystone_password": string(secret.Data[ServicePasswordSelector]),
		"openstack_cacert":       "",          // fixme
		"openstack_region_name":  "regionOne", // fixme
		"default_project_domain": "Default",   // fixme
		"default_user_domain":    "Default",   // fixme
		"transport_url":          string(cellMessageBusSecret.Data["transport_url"]),
		"nova_compute_image":     instance.Spec.NovaComputeContainerImage,
		"nova_libvirt_image":     instance.Spec.NovaLibvirtContainerImage,
		"log_file":               "/var/log/containers/nova/nova-compute.log",
		"novncproxy_base_url":    "http://" + vncHost, // fixme use https
	}
	extraData := map[string]string{}
	// always generate this file even if empty to simplify copying it
	// to the external compute.
	extraData["02-nova-override.conf"] = ""
	if instance.Spec.CustomServiceConfig != "" {
		extraData["02-nova-override.conf"] = instance.Spec.CustomServiceConfig
	}

	for key, data := range instance.Spec.DefaultConfigOverwrite {
		extraData[key] = data
	}

	cmLabels := labels.GetLabels(
		instance, labels.GetGroupLabel(NovaExternalComputeLabelPrefix), map[string]string{},
	)

	addtionalTemplates := map[string]string{
		"novacompute__nova_compute.json":                  "/novacompute/nova_compute.json",
		"novacompute__nova-compute.json":                  "/novacompute/nova-compute.json",
		"libvirt_virtsecretd__libvirt_virtsecretd.json":   "/libvirt_virtsecretd/libvirt_virtsecretd.json",
		"libvirt_virtsecretd__libvirt-virtsecretd.json":   "/libvirt_virtsecretd/libvirt-virtsecretd.json",
		"libvirt_virtsecretd__virtsecretd.conf":           "/libvirt_virtsecretd/virtsecretd.conf",
		"libvirt_virtqemud__libvirt_virtqemud.json":       "/libvirt_virtqemud/libvirt_virtqemud.json",
		"libvirt_virtqemud__libvirt-virtqemud.json":       "/libvirt_virtqemud/libvirt-virtqemud.json",
		"libvirt_virtqemud__virtqemud.conf":               "/libvirt_virtqemud/virtqemud.conf",
		"libvirt_virtqemud__qemu.conf":                    "/libvirt_virtqemud/qemu.conf",
		"libvirt_virtproxyd__libvirt_virtproxyd.json":     "/libvirt_virtproxyd/libvirt_virtproxyd.json",
		"libvirt_virtproxyd__libvirt-virtproxyd.json":     "/libvirt_virtproxyd/libvirt-virtproxyd.json",
		"libvirt_virtproxyd__virtproxyd.conf":             "/libvirt_virtproxyd/virtproxyd.conf",
		"libvirt_virtnodedevd__libvirt_virtnodedevd.json": "/libvirt_virtnodedevd/libvirt_virtnodedevd.json",
		"libvirt_virtnodedevd__libvirt-virtnodedevd.json": "/libvirt_virtnodedevd/libvirt-virtnodedevd.json",
		"libvirt_virtnodedevd__virtnodedevd.conf":         "/libvirt_virtnodedevd/virtnodedevd.conf",
		"libvirt_virtlogd__libvirt_virtlogd.json":         "/libvirt_virtlogd/libvirt_virtlogd.json",
		"libvirt_virtlogd__libvirt-virtlogd.json":         "/libvirt_virtlogd/libvirt-virtlogd.json",
		"libvirt_virtlogd__virtlogd.conf":                 "/libvirt_virtlogd/virtlogd.conf",
		"firewall.yaml":                                   "/firewall.goyaml",
	}
	return r.GenerateConfigs(
		ctx, h, instance, hashes, templateParameters, extraData, cmLabels, addtionalTemplates,
	)
}

// we do not include the input hash as the playbooks are not part of the CR input they
// are part of the operator state.
func (r *NovaExternalComputeReconciler) ensurePlaybooks(
	ctx context.Context, h *helper.Helper, instance *novav1.NovaExternalCompute,
) error {
	playbookPath, found := os.LookupEnv("OPERATOR_PLAYBOOKS")
	if !found {
		playbookPath = "playbooks"
		os.Setenv("OPERATOR_PLAYBOOKS", playbookPath)
		util.LogForObject(
			h, "OPERATOR_PLAYBOOKS not set in env when reconciling ", instance,
			"defaulting to ", playbookPath)
	}

	util.LogForObject(
		h, "using playbooks for instance ", instance, "from ", playbookPath,
	)
	playbookDirEntries, err := os.ReadDir(playbookPath)
	if err != nil {
		return err
	}
	// EnsureConfigMaps is not used as we do not want the templating behavior that adds.
	playbookCMName := fmt.Sprintf("%s-external-compute-playbooks", instance.Spec.NovaInstance)
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        playbookCMName,
			Namespace:   instance.Namespace,
			Annotations: map[string]string{},
		},
		Data: map[string]string{},
	}
	_, err = controllerutil.CreateOrPatch(ctx, h.GetClient(), configMap, func() error {

		// FIXME: This is wrong we should not be making the nova instance CR own this.
		// it should be owned by the operator... for now im just going to make sure
		// it exits and leak it it on operator removal.
		// novaInstance := &novav1.Nova{}
		// novaInstanceName := types.NamespacedName{
		// 	Namespace: instance.Namespace,
		// 	Name:      instance.Spec.NovaInstance,
		// }
		// err = h.GetClient().Get(ctx, novaInstanceName, novaInstance)
		// if err != nil {
		// 	util.LogForObject(
		// 		h, "Failed parent nova instance cr", instance,
		// 		"Nova ", instance.Spec.NovaInstance,
		// 	)
		// 	return err
		// }
		// configMap.Labels = labels.GetLabels(
		// 	novaInstance, labels.GetGroupLabel(NovaLabelPrefix), map[string]string{},
		// )
		// err = controllerutil.SetControllerReference(novaInstance, configMap, h.GetScheme())
		// if err != nil {
		// 	return err
		// }
		for _, entry := range playbookDirEntries {
			filename := entry.Name()
			filePath := path.Join(playbookPath, filename)
			data, err := os.ReadFile(filePath)
			if err != nil {
				return err
			}
			configMap.Data[filename] = string(data)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("error create/updating configmap: %w", err)
	}

	return nil
}

func (r *NovaExternalComputeReconciler) ensureAEEDeployLibvirt(
	ctx context.Context, h *helper.Helper, instance *novav1.NovaExternalCompute,
) (*aee.OpenStackAnsibleEE, error) {

	_labels := labels.GetLabels(
		instance, labels.GetGroupLabel(NovaExternalComputeLabelPrefix), map[string]string{},
	)
	ansibleEE := &aee.OpenStackAnsibleEE{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s-deploy-libvirt", instance.Spec.NovaInstance, instance.Name),
			Namespace: instance.Namespace,
			Labels:    _labels,
		},
	}

	aeeName := types.NamespacedName{
		Namespace: instance.Namespace,
		Name:      ansibleEE.Name,
	}
	err := h.GetClient().Get(ctx, aeeName, ansibleEE)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return ansibleEE, err
	}

	if k8s_errors.IsNotFound(err) {
		_, err = controllerutil.CreateOrPatch(ctx, h.GetClient(), ansibleEE, func() error {
			initAEE(instance, ansibleEE, "deploy-libvirt.yaml")

			return nil
		})

		if err != nil {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.DeploymentReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.DeploymentReadyErrorMessage,
				fmt.Errorf("during provisioning of libvirt: %w", err),
			))
			return ansibleEE, err
		}
	}
	return ansibleEE, nil
}

func (r *NovaExternalComputeReconciler) ensureAEEDeployNova(
	ctx context.Context, h *helper.Helper, instance *novav1.NovaExternalCompute,
) (*aee.OpenStackAnsibleEE, error) {

	_labels := labels.GetLabels(
		instance, labels.GetGroupLabel(NovaExternalComputeLabelPrefix), map[string]string{},
	)
	ansibleEE := &aee.OpenStackAnsibleEE{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s-deploy-nova", instance.Spec.NovaInstance, instance.Name),
			Namespace: instance.Namespace,
			Labels:    _labels,
		},
	}
	aeeName := types.NamespacedName{
		Namespace: instance.Namespace,
		Name:      ansibleEE.Name,
	}
	err := h.GetClient().Get(ctx, aeeName, ansibleEE)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return ansibleEE, err
	}

	if k8s_errors.IsNotFound(err) {
		_, err = controllerutil.CreateOrPatch(ctx, h.GetClient(), ansibleEE, func() error {
			initAEE(instance, ansibleEE, "deploy-nova.yaml")

			return nil
		})
		if err != nil {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.DeploymentReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.DeploymentReadyErrorMessage,
				fmt.Errorf("during provisioning of nova: %w", err),
			))
			return ansibleEE, err
		}
	}
	return ansibleEE, nil
}

func initAEE(
	instance *novav1.NovaExternalCompute, ansibleEE *aee.OpenStackAnsibleEE,
	playbook string,
) {
	ansibleEE.Spec.Image = instance.Spec.AnsibleEEContainerImage
	ansibleEE.Spec.DNSConfig = instance.Spec.DNSConfig
	ansibleEE.Spec.NetworkAttachments = instance.Spec.NetworkAttachments
	ansibleEE.Spec.Playbook = playbook
	ansibleEE.Spec.Env = []corev1.EnvVar{
		{Name: "ANSIBLE_FORCE_COLOR", Value: "True"},
		{Name: "ANSIBLE_SSH_ARGS", Value: "-C -o ControlMaster=auto -o ControlPersist=80s"},
		{Name: "ANSIBLE_ENABLE_TASK_DEBUGGER", Value: "True"},
		{Name: "ANSIBLE_VERBOSITY", Value: "1"},
	}
	// allocate temporary storage for the extra volume mounts to avoid
	// pointer indirection via ansibleEE.Spec.ExtraMounts
	ansibleEEMounts := storage.VolMounts{}

	// mount ssh keys
	sshKeyVolume := corev1.Volume{
		Name: "ssh-key",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: instance.Spec.SSHKeySecretName,
				Items: []corev1.KeyToPath{
					{
						Key:  "ssh-privatekey",
						Path: "ssh_key",
					},
				},
			},
		},
	}
	ansibleEEMounts.Volumes = append(ansibleEEMounts.Volumes, sshKeyVolume)
	sshKeyMount := corev1.VolumeMount{
		Name:      "ssh-key",
		MountPath: "/runner/env/ssh_key",
		SubPath:   "ssh_key",
	}
	ansibleEEMounts.Mounts = append(ansibleEEMounts.Mounts, sshKeyMount)

	// mount inventory
	inventoryVolume := corev1.Volume{
		Name: "inventory",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: instance.Spec.InventoryConfigMapName,
				},
				Items: []corev1.KeyToPath{
					{
						Key:  "inventory",
						Path: "inventory",
					},
				},
			},
		},
	}
	ansibleEEMounts.Volumes = append(ansibleEEMounts.Volumes, inventoryVolume)
	inventoryMount := corev1.VolumeMount{
		Name:      "inventory",
		MountPath: "/runner/inventory/hosts",
		SubPath:   "inventory",
	}
	ansibleEEMounts.Mounts = append(ansibleEEMounts.Mounts, inventoryMount)

	// mount nova playbooks
	playbookCMName := fmt.Sprintf("%s-external-compute-playbooks", instance.Spec.NovaInstance)
	playbookVolume := corev1.Volume{
		Name: "playbooks",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: playbookCMName,
				},
			},
		},
	}
	ansibleEEMounts.Volumes = append(ansibleEEMounts.Volumes, playbookVolume)
	playbookMount := corev1.VolumeMount{
		Name:      "playbooks",
		MountPath: fmt.Sprintf("/runner/project/%s", playbook),
		SubPath:   playbook,
	}
	ansibleEEMounts.Mounts = append(ansibleEEMounts.Mounts, playbookMount)

	// mount nova and libvirt configs
	serviceConfigSecretName := fmt.Sprintf("%s-config-data", instance.Name)
	serviceConfigVolume := corev1.Volume{
		Name: "compute-configs",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: serviceConfigSecretName,
			},
		},
	}
	ansibleEEMounts.Volumes = append(ansibleEEMounts.Volumes, serviceConfigVolume)
	serviceConfigMount := corev1.VolumeMount{
		Name:      "compute-configs",
		MountPath: "/var/lib/openstack/config",
	}
	ansibleEEMounts.Mounts = append(ansibleEEMounts.Mounts, serviceConfigMount)

	// initialize ansibleEE.Spec.ExtraMounts from local ansibleEEMounts
	ansibleEE.Spec.ExtraMounts = []storage.VolMounts{ansibleEEMounts}

}

func (r *NovaExternalComputeReconciler) cleanupAEE(
	ctx context.Context, h *helper.Helper, instance *aee.OpenStackAnsibleEE,
) error {
	err := h.GetClient().Delete(ctx, instance)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return err
	}
	return nil
}
