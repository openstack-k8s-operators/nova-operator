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

package cyborg

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rabbitmqv1 "github.com/openstack-k8s-operators/infra-operator/apis/rabbitmq/v1beta1"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/job"
	"github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	common_rbac "github.com/openstack-k8s-operators/lib-common/modules/common/rbac"
	"github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"

	cyborgv1beta1 "github.com/openstack-k8s-operators/nova-operator/api/cyborg/v1beta1"
	cyborgservice "github.com/openstack-k8s-operators/nova-operator/internal/cyborg"
)

// CyborgReconciler reconciles a Cyborg object.
//
//nolint:revive
type CyborgReconciler struct {
	ReconcilerBase
}

// GetLogger returns a logger object with a prefix of "controller.name" and additional controller context fields
func (r *CyborgReconciler) GetLogger(ctx context.Context) logr.Logger {
	return log.FromContext(ctx).WithName("Controllers").WithName("Cyborg")
}

// +kubebuilder:rbac:groups=cyborg.openstack.org,resources=cyborgs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cyborg.openstack.org,resources=cyborgs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cyborg.openstack.org,resources=cyborgs/finalizers,verbs=update
// +kubebuilder:rbac:groups=cyborg.openstack.org,resources=cyborgapis,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cyborg.openstack.org,resources=cyborgapis/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cyborg.openstack.org,resources=cyborgapis/finalizers,verbs=update
// +kubebuilder:rbac:groups=cyborg.openstack.org,resources=cyborgconductors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cyborg.openstack.org,resources=cyborgconductors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cyborg.openstack.org,resources=cyborgconductors/finalizers,verbs=update
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbaccounts/finalizers,verbs=update
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbdatabases,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=rabbitmq.openstack.org,resources=transporturls,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneservices,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneendpoints,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups="",resources=pods,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=roles,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="security.openshift.io",resourceNames=anyuid,resources=securitycontextconstraints,verbs=use

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *CyborgReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, _err error) {
	Log := r.GetLogger(ctx)

	instance := &cyborgv1beta1.Cyborg{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected.
			// For additional cleanup logic use finalizers. Return and don't requeue.
			Log.Info("Cyborg instance not found, probably deleted before reconciled. Nothing to do.")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		Log.Error(err, "Failed to read the Cyborg instance.")
		return ctrl.Result{}, err
	}

	Log.Info(fmt.Sprintf("Reconciling Cyborg instance '%s'", instance.Name))

	h, err := helper.NewHelper(
		instance,
		r.Client,
		r.Kclient,
		r.Scheme,
		Log,
	)
	if err != nil {
		Log.Error(err, "Failed to create lib-common Helper")
		return ctrl.Result{}, err
	}

	serviceLabels := map[string]string{
		common.AppSelector: cyborgservice.ServiceName,
	}

	isNewInstance := instance.Status.Conditions == nil
	savedConditions := instance.Status.Conditions.DeepCopy()

	defer func() {
		if r := recover(); r != nil {
			Log.Info(fmt.Sprintf("panic during reconcile %v\n", r))
			panic(r)
		}

		// Update the Ready condition based on the sub conditions
		if instance.Status.Conditions.AllSubConditionIsTrue() {
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

		condition.RestoreLastTransitionTimes(&instance.Status.Conditions, savedConditions)
		err := h.PatchInstance(ctx, instance)
		if err != nil {
			_err = err
			return
		}
	}()

	// Initialize the status of the instance, including the conditions, hash, and observed generation.
	err = r.initStatus(instance)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Handle service delete
	if !instance.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, instance, h)
	}

	err = r.ensureRbac(ctx, h, instance)
	if err != nil {
		return ctrl.Result{}, err
	}

	if instance.DeletionTimestamp.IsZero() && controllerutil.AddFinalizer(instance, h.GetFinalizer()) || isNewInstance {
		return ctrl.Result{}, nil
	}

	//
	// Create the DB and required DB account
	//
	db, result, err := r.ensureDB(ctx, h, instance)
	if err != nil {
		return ctrl.Result{}, err
	} else if (result != ctrl.Result{}) {
		return result, nil
	}

	//
	// Create RabbitMQ TransportURL
	//
	transportURL, op, err := r.ensureMQ(ctx, instance, h, instance.Name+"-cyborg-transport", instance.Spec.MessagingBus, serviceLabels)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			cyborgv1beta1.CyborgRabbitMQTransportURLReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			cyborgv1beta1.CyborgRabbitMQTransportURLReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	if transportURL == nil {
		Log.Info(fmt.Sprintf("Waiting for TransportURL for %s to be created", instance.Name))
		instance.Status.Conditions.Set(condition.FalseCondition(
			cyborgv1beta1.CyborgRabbitMQTransportURLReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			cyborgv1beta1.CyborgRabbitMQTransportURLReadyRunningMessage))
		return ctrl.Result{RequeueAfter: time.Duration(10) * time.Second}, nil
	}

	instance.Status.Conditions.MarkTrue(
		cyborgv1beta1.CyborgRabbitMQTransportURLReadyCondition,
		cyborgv1beta1.CyborgRabbitMQTransportURLReadyMessage)

	_ = op

	//
	// Validate input secret (password secret)
	//
	hash, _, inputSecret, err := ensureSecret(
		ctx,
		types.NamespacedName{Namespace: instance.Namespace, Name: *instance.Spec.Secret},
		[]string{
			instance.Spec.PasswordSelectors.Service,
		},
		h.GetClient(),
		&instance.Status.Conditions,
		r.RequeueTimeout,
	)
	if err != nil || hash == "" {
		return ctrl.Result{}, ErrRetrievingSecretData
	}

	// TransportURL Secret
	hashTransporturl, _, transporturlSecret, err := ensureSecret(
		ctx,
		types.NamespacedName{Namespace: instance.Namespace, Name: transportURL.Status.SecretName},
		[]string{
			TransportURLSelector,
		},
		h.GetClient(),
		&instance.Status.Conditions,
		r.RequeueTimeout,
	)
	if err != nil || hashTransporturl == "" {
		return ctrl.Result{}, ErrRetrievingTransportURLSecretData
	}

	//
	// Handle Application Credentials
	//
	var acData *keystonev1.ApplicationCredentialData
	if instance.Spec.Auth.ApplicationCredentialSecret != "" {
		acSecretObj, _, err := secret.GetSecret(ctx, h, instance.Spec.Auth.ApplicationCredentialSecret, instance.Namespace)
		if err != nil {
			if k8s_errors.IsNotFound(err) {
				Log.Info("ApplicationCredential secret not found, waiting", "secret", instance.Spec.Auth.ApplicationCredentialSecret)
				instance.Status.Conditions.Set(condition.FalseCondition(
					condition.InputReadyCondition,
					condition.RequestedReason,
					condition.SeverityInfo,
					cyborgv1beta1.CyborgApplicationCredentialSecretErrorMessage))
				return ctrl.Result{}, fmt.Errorf("%w: %s", ErrACSecretNotFound, instance.Spec.Auth.ApplicationCredentialSecret)
			}
			Log.Error(err, "Failed to get ApplicationCredential secret", "secret", instance.Spec.Auth.ApplicationCredentialSecret)
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.InputReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				cyborgv1beta1.CyborgApplicationCredentialSecretErrorMessage))
			return ctrl.Result{}, err
		}
		acID, okID := acSecretObj.Data[keystonev1.ACIDSecretKey]
		acSecretData, okSecret := acSecretObj.Data[keystonev1.ACSecretSecretKey]
		if okID && len(acID) > 0 && okSecret && len(acSecretData) > 0 {
			acData = &keystonev1.ApplicationCredentialData{
				ID:     string(acID),
				Secret: string(acSecretData),
			}
			Log.Info("Using ApplicationCredentials auth", "secret", instance.Spec.Auth.ApplicationCredentialSecret)
		} else {
			Log.Error(nil, "ApplicationCredential secret missing required keys", "secret", instance.Spec.Auth.ApplicationCredentialSecret)
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.InputReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				cyborgv1beta1.CyborgApplicationCredentialSecretErrorMessage))
			return ctrl.Result{}, fmt.Errorf("%w: %s", ErrACSecretMissingKeys, instance.Spec.Auth.ApplicationCredentialSecret)
		}
	}

	//
	// Create sub-level secret with required configuration
	//
	_, err = r.createSubLevelSecret(ctx, h, instance, transporturlSecret, inputSecret, db, acData)
	if err != nil {
		return ctrl.Result{}, err
	}

	instance.Status.Conditions.MarkTrue(condition.InputReadyCondition, condition.InputReadyMessage)

	//
	// Create Keystone Service
	//
	_, err = r.ensureKeystoneSvc(ctx, h, instance, serviceLabels)
	if err != nil {
		return ctrl.Result{}, err
	}

	//
	// Generate config for dbsync
	//
	configVars := make(map[string]env.Setter)

	err = r.generateServiceConfig(ctx, instance, db, h, &configVars)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ServiceConfigReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceConfigReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	instance.Status.Conditions.MarkTrue(condition.ServiceConfigReadyCondition, condition.ServiceConfigReadyMessage)

	//
	// Create dbsync job
	//
	ctrlResult, err := r.ensureDBSync(ctx, h, instance, serviceLabels)
	if err != nil {
		return ctrl.Result{}, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	//
	// Remove finalizers from unused MariaDBAccount records
	//
	err = mariadbv1.DeleteUnusedMariaDBAccountFinalizers(
		ctx, h, cyborgservice.DatabaseCRName,
		*instance.Spec.DatabaseAccount, instance.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	if instance.Status.Conditions.AllSubConditionIsTrue() {
		instance.Status.Conditions.MarkTrue(
			condition.ReadyCondition, condition.ReadyMessage)
	}

	return ctrl.Result{}, nil
}

func (r *CyborgReconciler) initStatus(instance *cyborgv1beta1.Cyborg) error {
	err := r.initConditions(instance)
	if err != nil {
		return err
	}

	instance.Status.ObservedGeneration = instance.Generation

	if instance.Status.Hash == nil {
		instance.Status.Hash = make(map[string]string)
	}

	return nil
}

func (r *CyborgReconciler) initConditions(instance *cyborgv1beta1.Cyborg) error {
	if instance.Status.Conditions == nil {
		instance.Status.Conditions = condition.Conditions{}
	}

	cl := condition.CreateList(
		condition.UnknownCondition(condition.ReadyCondition, condition.InitReason, condition.ReadyInitMessage),
		condition.UnknownCondition(condition.DBReadyCondition, condition.InitReason, condition.DBReadyInitMessage),
		condition.UnknownCondition(
			cyborgv1beta1.CyborgRabbitMQTransportURLReadyCondition,
			condition.InitReason,
			condition.RabbitMqTransportURLReadyInitMessage),
		condition.UnknownCondition(
			condition.InputReadyCondition,
			condition.InitReason,
			condition.InputReadyInitMessage),
		condition.UnknownCondition(
			condition.KeystoneServiceReadyCondition,
			condition.InitReason,
			"Service registration not started"),
		condition.UnknownCondition(
			condition.ServiceAccountReadyCondition,
			condition.InitReason,
			condition.ServiceAccountReadyInitMessage),
		condition.UnknownCondition(
			condition.RoleReadyCondition,
			condition.InitReason,
			condition.RoleReadyInitMessage),
		condition.UnknownCondition(
			condition.RoleBindingReadyCondition,
			condition.InitReason,
			condition.RoleBindingReadyInitMessage),
		condition.UnknownCondition(
			condition.ServiceConfigReadyCondition,
			condition.InitReason,
			condition.ServiceConfigReadyInitMessage),
		condition.UnknownCondition(
			condition.DBSyncReadyCondition,
			condition.InitReason,
			condition.DBSyncReadyInitMessage),
	)

	instance.Status.Conditions.Init(&cl)

	return nil
}

func (r *CyborgReconciler) ensureRbac(
	ctx context.Context,
	h *helper.Helper,
	instance *cyborgv1beta1.Cyborg,
) error {
	rbacRules := []rbacv1.PolicyRule{
		{
			APIGroups:     []string{"security.openshift.io"},
			ResourceNames: []string{"anyuid"},
			Resources:     []string{"securitycontextconstraints"},
			Verbs:         []string{"use"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"pods"},
			Verbs:     []string{"create", "get", "list", "watch", "update", "patch", "delete"},
		},
	}

	_, err := common_rbac.ReconcileRbac(ctx, h, instance, rbacRules)
	if err != nil {
		return err
	}

	return nil
}

func (r *CyborgReconciler) ensureDB(
	ctx context.Context,
	h *helper.Helper,
	instance *cyborgv1beta1.Cyborg,
) (*mariadbv1.Database, ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info(fmt.Sprintf("Reconciling the DB instance for '%s'", instance.Name))

	_, _, err := mariadbv1.EnsureMariaDBAccount(
		ctx, h, *instance.Spec.DatabaseAccount,
		instance.Namespace, false, cyborgservice.DatabaseUsernamePrefix,
	)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			mariadbv1.MariaDBAccountReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			mariadbv1.MariaDBAccountNotReadyMessage,
			err.Error()))
		return nil, ctrl.Result{}, err
	}
	instance.Status.Conditions.MarkTrue(
		mariadbv1.MariaDBAccountReadyCondition,
		mariadbv1.MariaDBAccountReadyMessage)

	db := mariadbv1.NewDatabaseForAccount(
		*instance.Spec.DatabaseInstance,
		cyborgservice.DatabaseName,
		cyborgservice.DatabaseCRName,
		*instance.Spec.DatabaseAccount,
		instance.Namespace,
	)

	ctrlResult, err := db.CreateOrPatchAll(ctx, h)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DBReadyErrorMessage,
			err.Error()))
		return db, ctrl.Result{}, err
	}
	if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DBReadyRunningMessage))
		return db, ctrlResult, nil
	}

	ctrlResult, err = db.WaitForDBCreatedWithTimeout(ctx, h, r.RequeueTimeout)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DBReadyErrorMessage,
			err.Error()))
		return db, ctrlResult, err
	}
	if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DBReadyRunningMessage))
		return db, ctrlResult, nil
	}

	instance.Status.Conditions.MarkTrue(condition.DBReadyCondition, condition.DBReadyMessage)

	return db, ctrl.Result{}, err
}

func (r *CyborgReconciler) ensureMQ(
	ctx context.Context,
	instance *cyborgv1beta1.Cyborg,
	h *helper.Helper,
	transportURLName string,
	rabbitMqConfig rabbitmqv1.RabbitMqConfig,
	serviceLabels map[string]string,
) (*rabbitmqv1.TransportURL, controllerutil.OperationResult, error) {
	Log := r.GetLogger(ctx)
	Log.Info(fmt.Sprintf("Reconciling the RabbitMQ TransportURL '%s' for '%s'", transportURLName, instance.Name))

	transportURL := &rabbitmqv1.TransportURL{
		ObjectMeta: metav1.ObjectMeta{
			Name:      transportURLName,
			Namespace: instance.Namespace,
			Labels:    serviceLabels,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, transportURL, func() error {
		transportURL.Spec.RabbitmqClusterName = rabbitMqConfig.Cluster
		transportURL.Spec.Username = rabbitMqConfig.User
		transportURL.Spec.Vhost = rabbitMqConfig.Vhost

		err := controllerutil.SetControllerReference(instance, transportURL, r.Scheme)
		return err
	})

	if err != nil && !k8s_errors.IsNotFound(err) {
		return nil, op, util.WrapErrorForObject(
			fmt.Sprintf("error creating or updating TransportURL object %s", transportURLName),
			transportURL,
			err,
		)
	}

	if op != controllerutil.OperationResultNone {
		Log.Info(fmt.Sprintf("TransportURL %s successfully reconciled - operation: %s", transportURL.Name, string(op)))
	}

	if !transportURL.IsReady() || transportURL.Status.SecretName == "" {
		Log.Info(fmt.Sprintf("Waiting for TransportURL %s secret to be created", transportURL.Name))
		return nil, op, nil
	}

	secretName := types.NamespacedName{Namespace: instance.Namespace, Name: transportURL.Status.SecretName}
	transportSecret := &corev1.Secret{}
	err = h.GetClient().Get(ctx, secretName, transportSecret)
	if err != nil {
		return nil, op, err
	}

	_, ok := transportSecret.Data[TransportURLSelector]
	if !ok {
		return nil, op, fmt.Errorf("%w: %s", ErrTransportURLFieldMissing, transportURL.Status.SecretName)
	}

	return transportURL, op, nil
}

func (r *CyborgReconciler) ensureKeystoneSvc(
	ctx context.Context,
	h *helper.Helper,
	instance *cyborgv1beta1.Cyborg,
	serviceLabels map[string]string,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info(fmt.Sprintf("Reconciling the Keystone Service for '%s'", instance.Name))

	ksSvcSpec := keystonev1.KeystoneServiceSpec{
		ServiceType:        cyborgservice.ServiceType,
		ServiceName:        cyborgservice.ServiceName,
		ServiceDescription: "Cyborg Accelerator Lifecycle Management Service",
		Enabled:            true,
		ServiceUser:        *instance.Spec.ServiceUser,
		Secret:             *instance.Spec.Secret,
		PasswordSelector:   instance.Spec.PasswordSelectors.Service,
	}

	ksSvc := keystonev1.NewKeystoneService(ksSvcSpec, instance.Namespace, serviceLabels, time.Duration(10)*time.Second)
	ctrlResult, err := ksSvc.CreateOrPatch(ctx, h)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.KeystoneServiceReadyCondition,
			condition.CreationFailedReason,
			condition.SeverityError,
			"Error while creating Keystone Service for Cyborg"))
		return ctrlResult, err
	}

	c := ksSvc.GetConditions().Mirror(condition.KeystoneServiceReadyCondition)
	if c != nil {
		instance.Status.Conditions.Set(c)
	}

	if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	instance.Status.ServiceID = ksSvc.GetServiceID()

	return ctrlResult, nil
}

func (r *CyborgReconciler) createSubLevelSecret(
	ctx context.Context,
	h *helper.Helper,
	instance *cyborgv1beta1.Cyborg,
	transportURLSecret corev1.Secret,
	inputSecret corev1.Secret,
	db *mariadbv1.Database,
	acData *keystonev1.ApplicationCredentialData,
) (string, error) {
	Log := r.GetLogger(ctx)
	Log.Info(fmt.Sprintf("Creating SubCR Level Secret for '%s'", instance.Name))

	databaseAccount := db.GetAccount()
	databaseSecret := db.GetSecret()

	data := map[string]string{
		instance.Spec.PasswordSelectors.Service: string(inputSecret.Data[instance.Spec.PasswordSelectors.Service]),
		TransportURLSelector:                    string(transportURLSecret.Data[TransportURLSelector]),
		QuorumQueuesSelector:                    string(transportURLSecret.Data[QuorumQueuesSelector]),
		DatabaseAccount:                         databaseAccount.Name,
		DatabaseUsername:                        databaseAccount.Spec.UserName,
		DatabasePassword:                        string(databaseSecret.Data[mariadbv1.DatabasePasswordSelector]),
		DatabaseHostname:                        db.GetDatabaseHostname(),
	}

	if acData != nil {
		data["ACID"] = acData.ID
		data["ACSecret"] = acData.Secret
	}

	secretName := instance.Name
	serviceLabels := labels.GetLabels(instance, labels.GetGroupLabel(cyborgservice.ServiceName), map[string]string{})

	template := util.Template{
		Name:         secretName,
		Namespace:    instance.Namespace,
		Type:         util.TemplateTypeNone,
		InstanceType: instance.GetObjectKind().GroupVersionKind().Kind,
		Labels:       serviceLabels,
		CustomData:   data,
	}

	err := secret.EnsureSecrets(ctx, h, instance, []util.Template{template}, nil)

	return secretName, err
}

func (r *CyborgReconciler) generateServiceConfig(
	ctx context.Context,
	instance *cyborgv1beta1.Cyborg,
	db *mariadbv1.Database,
	h *helper.Helper,
	envVars *map[string]env.Setter,
) error {
	Log := r.GetLogger(ctx)
	Log.Info("generateServiceConfig - reconciling config for Cyborg CR")

	var tlsCfg *tls.Service
	if instance.Spec.APIServiceTemplate.TLS.CaBundleSecretName != "" {
		tlsCfg = &tls.Service{}
	}

	databaseAccount := db.GetAccount()
	databaseSecret := db.GetSecret()

	templateParameters := map[string]any{
		"DatabaseConnection": fmt.Sprintf("mysql+pymysql://%s:%s@%s/%s?read_default_file=/etc/my.cnf",
			databaseAccount.Spec.UserName,
			string(databaseSecret.Data[mariadbv1.DatabasePasswordSelector]),
			db.GetDatabaseHostname(),
			cyborgservice.DatabaseName,
		),
	}

	customData := map[string]string{
		"my.cnf": db.GetDatabaseClientConfig(tlsCfg),
	}

	serviceLabels := labels.GetLabels(instance, labels.GetGroupLabel(cyborgservice.ServiceName), map[string]string{})

	cms := []util.Template{
		{
			Name:          fmt.Sprintf("%s-config-data", instance.GetName()),
			Namespace:     instance.GetNamespace(),
			Type:          util.TemplateTypeConfig,
			InstanceType:  instance.GetObjectKind().GroupVersionKind().Kind,
			ConfigOptions: templateParameters,
			CustomData:    customData,
			Labels:        serviceLabels,
			AdditionalTemplate: map[string]string{
				"00-default.conf":           "/cyborg/00-default.conf",
				"cyborg-dbsync-config.json": "/cyborg/cyborg/cyborg-dbsync-config.json",
			},
		},
	}

	return secret.EnsureSecrets(ctx, h, instance, cms, envVars)
}

func (r *CyborgReconciler) ensureDBSync(
	ctx context.Context,
	h *helper.Helper,
	instance *cyborgv1beta1.Cyborg,
	serviceLabels map[string]string,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info(fmt.Sprintf("Reconciling the DB Sync for '%s'", instance.Name))

	dbSyncHash := instance.Status.Hash[cyborgv1beta1.DbSyncHash]
	jobDef := cyborgservice.DbSyncJob(instance, serviceLabels, nil)

	dbSyncjob := job.NewJob(
		jobDef,
		cyborgv1beta1.DbSyncHash,
		instance.Spec.PreserveJobs,
		time.Duration(5)*time.Second,
		dbSyncHash,
	)

	ctrlResult, err := dbSyncjob.DoJob(ctx, h)

	if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBSyncReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DBSyncReadyRunningMessage))
		return ctrlResult, nil
	}
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBSyncReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DBSyncReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	if dbSyncjob.HasChanged() {
		instance.Status.Hash[cyborgv1beta1.DbSyncHash] = dbSyncjob.GetHash()
		Log.Info(fmt.Sprintf("Service '%s' - Job %s hash added - %s", instance.Name, jobDef.Name, instance.Status.Hash[cyborgv1beta1.DbSyncHash]))
	}
	instance.Status.Conditions.MarkTrue(condition.DBSyncReadyCondition, condition.DBSyncReadyMessage)

	return ctrlResult, nil
}

func (r *CyborgReconciler) reconcileDelete(
	ctx context.Context,
	instance *cyborgv1beta1.Cyborg,
	h *helper.Helper,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info(fmt.Sprintf("Reconcile Service '%s' delete started", instance.Name))

	err := mariadbv1.DeleteDatabaseAndAccountFinalizers(ctx, h, cyborgservice.DatabaseCRName, *instance.Spec.DatabaseAccount, instance.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	Log.Info("Removed finalizer from MariaDBDatabase CR", "MariaDBDatabase name", cyborgservice.DatabaseCRName)

	keystoneService, err := keystonev1.GetKeystoneServiceWithName(ctx, h, cyborgservice.ServiceName, instance.Namespace)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	if err == nil {
		if controllerutil.RemoveFinalizer(keystoneService, h.GetFinalizer()) {
			err = h.GetClient().Update(ctx, keystoneService)
			if err != nil && !k8s_errors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
			util.LogForObject(h, "Removed finalizer from our KeystoneService", instance)
		}
	}

	// Successfully cleaned up everything. So as the final step let's remove the
	// finalizer from ourselves to allow the deletion of Nova CR itself
	updated := controllerutil.RemoveFinalizer(instance, h.GetFinalizer())
	if updated {
		Log.Info("Removed finalizer from ourselves")
	}

	Log.Info(fmt.Sprintf("Reconciled Service '%s' delete successfully", instance.Name))
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CyborgReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &cyborgv1beta1.Cyborg{}, passwordSecretField, func(rawObj client.Object) []string {
		cr := rawObj.(*cyborgv1beta1.Cyborg)
		if cr.Spec.Secret == nil || *cr.Spec.Secret == "" {
			return nil
		}
		return []string{*cr.Spec.Secret}
	}); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &cyborgv1beta1.Cyborg{}, authAppCredSecretField, func(rawObj client.Object) []string {
		cr := rawObj.(*cyborgv1beta1.Cyborg)
		if cr.Spec.Auth.ApplicationCredentialSecret == "" {
			return nil
		}
		return []string{cr.Spec.Auth.ApplicationCredentialSecret}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&cyborgv1beta1.Cyborg{}).
		Owns(&mariadbv1.MariaDBDatabase{}).
		Owns(&mariadbv1.MariaDBAccount{}).
		Owns(&rabbitmqv1.TransportURL{}).
		Owns(&keystonev1.KeystoneService{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&batchv1.Job{}).
		Owns(&corev1.Secret{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForSrc),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Named("cyborg-cyborg").
		Complete(r)
}

func (r *CyborgReconciler) findObjectsForSrc(ctx context.Context, src client.Object) []reconcile.Request {
	var requests []reconcile.Request
	Log := r.GetLogger(ctx)

	for _, field := range cyborgWatchFields {
		crList := &cyborgv1beta1.CyborgList{}
		listOps := &client.ListOptions{
			FieldSelector: fields.OneTermEqualSelector(field, src.GetName()),
			Namespace:     src.GetNamespace(),
		}
		err := r.Client.List(ctx, crList, listOps)
		if err != nil {
			Log.Error(err, fmt.Sprintf("listing %s for field: %s - %s", crList.GroupVersionKind().Kind, field, src.GetNamespace()))
			return requests
		}

		for _, item := range crList.Items {
			Log.Info(fmt.Sprintf("input source %s changed, reconcile: %s - %s", src.GetName(), item.GetName(), item.GetNamespace()))
			requests = append(requests,
				reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      item.GetName(),
						Namespace: item.GetNamespace(),
					},
				},
			)
		}
	}

	return requests
}

func ensureSecret(
	ctx context.Context,
	secretName types.NamespacedName,
	expectedFields []string,
	reader client.Reader,
	conditionUpdater conditionUpdater,
	requeueTimeout time.Duration,
) (string, ctrl.Result, corev1.Secret, error) {
	s := &corev1.Secret{}
	err := reader.Get(ctx, secretName, s)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			log.FromContext(ctx).Info(fmt.Sprintf("secret %s not found", secretName))
			conditionUpdater.Set(condition.FalseCondition(
				condition.InputReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.InputReadyWaitingMessage))
			return "",
				ctrl.Result{RequeueAfter: requeueTimeout},
				*s,
				nil
		}
		conditionUpdater.Set(condition.FalseCondition(
			condition.InputReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.InputReadyErrorMessage,
			err.Error()))
		return "", ctrl.Result{}, *s, err
	}

	var values [][]byte
	for _, field := range expectedFields {
		val, ok := s.Data[field]
		if !ok {
			err := fmt.Errorf("%w: '%s' in secret/%s", ErrSecretFieldNotFound, field, secretName.Name)
			conditionUpdater.Set(condition.FalseCondition(
				condition.InputReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.InputReadyErrorMessage,
				err.Error()))
			return "", ctrl.Result{}, *s, err
		}
		values = append(values, val)
	}

	hash, err := util.ObjectHash(values)
	if err != nil {
		conditionUpdater.Set(condition.FalseCondition(
			condition.InputReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.InputReadyErrorMessage,
			err.Error()))
		return "", ctrl.Result{}, *s, err
	}

	return hash, ctrl.Result{}, *s, nil
}
