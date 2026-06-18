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

// Package controller contains the Nova operator controllers for managing OpenStack Nova services.
package controller

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	memcachedv1 "github.com/openstack-k8s-operators/infra-operator/apis/memcached/v1beta1"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/nova/v1beta1"
	internalcommon "github.com/openstack-k8s-operators/nova-operator/internal/common"

	gophercloud "github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/services"
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	util "github.com/openstack-k8s-operators/lib-common/modules/common/util"
	"github.com/openstack-k8s-operators/lib-common/modules/openstack"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// NovaAPILabelPrefix - a unique, service binary specific prefix for the
	// labels the NovaAPI controller uses on children objects
	NovaAPILabelPrefix = "nova-api"
	// NovaConductorLabelPrefix - a unique, service binary specific prefix for
	// the labels the NovaConductor controller uses on children objects
	NovaConductorLabelPrefix = "nova-conductor"
	// NovaSchedulerLabelPrefix - a unique, service binary specific prefix for
	// the labels the NovaScheduler controller uses on children objects
	NovaSchedulerLabelPrefix = "nova-scheduler"
	// NovaCellLabelPrefix - a unique, prefix used for the compute config
	// Secret
	NovaCellLabelPrefix = "nova-cell"
	// NovaLabelPrefix - a unique, prefix used for labels on Nova CR level jobs
	// and Secrets
	NovaLabelPrefix = "nova"
	// NovaMetadataLabelPrefix - a unique, service binary specific prefix for
	// the labels the NovaMetadata controller uses on children objects
	NovaMetadataLabelPrefix = "nova-metadata"
	// NovaNoVNCProxyLabelPrefix - a unique, service binary specific prefix for
	// the labels the NovaNoVNCProxy controller uses on children objects
	NovaNoVNCProxyLabelPrefix = "nova-novncproxy"
	// NovaComputeLabelPrefix - a unique, service binary specific prefix for
	// the labels the nova-compute controller uses on children objects
	NovaComputeLabelPrefix = "nova-compute"
	// DbSyncHash - the field name in Status.Hashes storing the has of the DB
	// sync job
	DbSyncHash = "dbsync"
	// CellSelector is the key name of a cell label
	CellSelector = "cell"

	// ServicePasswordSelector is the name of key in the internal Secret for
	// the nova service password
	ServicePasswordSelector = "ServicePassword"
	// MetadataSecretSelector is the name of key in the internal Secret for
	// the metadata shared secret
	MetadataSecretSelector = "MetadataSecret"
	// TransportURLSelector is the name of key in the internal cell
	// Secret for the cell message bus transport URL
	TransportURLSelector = "transport_url"

	// NotificationTransportURLSelector is the name of
	// top level notification message bus transport URL
	NotificationTransportURLSelector = "notification_transport_url"

	// QuorumQueuesSelector is the name of key in the TransportURL Secret for
	// the message bus quorum queues
	QuorumQueuesSelector = "quorumqueues"

	// QuorumQueuesTemplateKey is the name of key in template parameters for
	// the message bus quorum queues configuration
	QuorumQueuesTemplateKey = "quorum_queues"

	// RabbitmqUserNameSelector is the name of key in the internal Secret for
	// the RabbitMQUser CR name for the RPC/messaging bus
	RabbitmqUserNameSelector = "rabbitmq_user_name"

	// NotificationRabbitmqUserNameSelector is the name of key in the internal
	// Secret for the RabbitMQUser CR name for the notifications bus
	NotificationRabbitmqUserNameSelector = "notification_rabbitmq_user_name"

	// fields to index to reconcile when change
	passwordSecretField        = ".spec.secret"
	authAppCredSecretField     = ".spec.auth.applicationCredentialSecret" // #nosec G101
	caBundleSecretNameField    = ".spec.tls.caBundleSecretName"           // #nosec G101
	tlsAPIInternalField        = ".spec.tls.api.internal.secretName"
	tlsAPIPublicField          = ".spec.tls.api.public.secretName"
	tlsMetadataField           = ".spec.tls.secretName"
	tlsNoVNCProxyServiceField  = ".spec.tls.service.secretName"
	tlsNoVNCProxyVencryptField = ".spec.tls.vencrypt.secretName"
	topologyField              = ".spec.topologyRef.Name"

	// NovaAPIDatabaseName is the name of the DB schema created for the
	// top level nova DB
	NovaAPIDatabaseName = "nova_api"
	// NovaCell0DatabaseName - the name of the DB to store the cell schema for
	// cell0
	NovaCell0DatabaseName = "nova_cell0"

	// services nova depends, which are endpoint AppSelector labels
	// (mschuppert) - Manial should be added starting Dalmatian
	endpointBarbican  = "barbican"
	endpointCinder    = "cinder"
	endpointGlance    = "glance"
	endpointNeutron   = "neutron"
	endpointPlacement = "placement"
)

var (
	endpointList = []string{
		endpointBarbican,
		endpointCinder,
		endpointGlance,
		endpointNeutron,
		endpointPlacement,
	}

	novaDefaultExtraTemplates = map[string]string{
		"01-nova.conf":    "/nova/nova.conf",
		"nova-blank.conf": "/nova/nova-blank.conf",
	}

	// ErrACSecretMissingKeys indicates that the ApplicationCredential secret is missing required keys
	ErrACSecretMissingKeys = internalcommon.ErrACSecretMissingKeys
)

type conditionsGetter = internalcommon.ConditionsGetter

type topologyHandler = internalcommon.TopologyHandler

type conditionUpdater = internalcommon.ConditionUpdater

// ReconcilerBase provides a common set of clients scheme and loggers for all reconcilers.
type ReconcilerBase struct {
	internalcommon.ReconcilerBase
}

// Reconcilers holds all the Reconciler objects of the nova-operator.
type Reconcilers = internalcommon.Reconcilers

// Reconciler represents a generic interface for all Reconciler objects in nova.
type Reconciler = internalcommon.Reconciler

// NewReconcilerBase constructs a ReconcilerBase given a manager and Kclient.
func NewReconcilerBase(
	mgr ctrl.Manager, kclient kubernetes.Interface,
) ReconcilerBase {
	return ReconcilerBase{ReconcilerBase: internalcommon.NewReconcilerBase(mgr, kclient)}
}

// NewReconcilers constructs all nova Reconciler objects
func NewReconcilers(mgr ctrl.Manager, kclient *kubernetes.Clientset) *Reconcilers {
	return internalcommon.NewReconcilersRegistry(map[string]Reconciler{
		"Nova": &NovaReconciler{
			ReconcilerBase: NewReconcilerBase(mgr, kclient),
		},
		"NovaCell": &NovaCellReconciler{
			ReconcilerBase: NewReconcilerBase(mgr, kclient),
		},
		"NovaAPI": &NovaAPIReconciler{
			ReconcilerBase: NewReconcilerBase(mgr, kclient),
		},
		"NovaScheduler": &NovaSchedulerReconciler{
			ReconcilerBase: NewReconcilerBase(mgr, kclient),
		},
		"NovaConductor": &NovaConductorReconciler{
			ReconcilerBase: NewReconcilerBase(mgr, kclient),
		},
		"NovaMetadata": &NovaMetadataReconciler{
			ReconcilerBase: NewReconcilerBase(mgr, kclient),
		},
		"NovaNoVNCProxy": &NovaNoVNCProxyReconciler{
			ReconcilerBase: NewReconcilerBase(mgr, kclient),
		},
		"NovaCompute": &NovaComputeReconciler{
			ReconcilerBase: NewReconcilerBase(mgr, kclient),
		},
	})
}

// GenerateConfigs helper function to generate config maps
func (r *ReconcilerBase) GenerateConfigs(
	ctx context.Context, h *helper.Helper,
	instance client.Object, configName string, envVars *map[string]env.Setter,
	templateParameters map[string]any,
	extraData map[string]string, cmLabels map[string]string,
	additionalTemplates map[string]string,
	commonTemplates []string,
	templateDir string,
) error {
	return r.ReconcilerBase.GenerateConfigs(ctx, h, instance, envVars, internalcommon.ConfigGeneratorOptions{
		ConfigName:          configName,
		TemplateDir:         templateDir,
		BaseExtraTemplates:  novaDefaultExtraTemplates,
		AdditionalTemplates: additionalTemplates,
		CommonTemplates:     commonTemplates,
		TemplateParameters:  templateParameters,
		ExtraData:           extraData,
		Labels:              cmLabels,
	})
}

// GenerateConfigsWithScripts helper function to generate config maps
// for service configs and scripts
func (r *ReconcilerBase) GenerateConfigsWithScripts(
	ctx context.Context, h *helper.Helper,
	instance client.Object, envVars *map[string]env.Setter,
	templateParameters map[string]any,
	extraData map[string]string, cmLabels map[string]string,
	additionalTemplates map[string]string,
	commonTemplates []string,
	templateDir string,
) error {
	return r.ReconcilerBase.GenerateConfigs(ctx, h, instance, envVars, internalcommon.ConfigGeneratorOptions{
		ConfigName:          internalcommon.GetServiceConfigSecretName(instance.GetName()),
		TemplateDir:         templateDir,
		BaseExtraTemplates:  novaDefaultExtraTemplates,
		AdditionalTemplates: additionalTemplates,
		CommonTemplates:     commonTemplates,
		TemplateParameters:  templateParameters,
		ExtraData:           extraData,
		Labels:              cmLabels,
		WithScripts:         true,
	})
}

// GetSecret defines an interface for objects that can provide a secret name
type GetSecret = internalcommon.GetSecret

func ensureTopology(
	ctx context.Context,
	h *helper.Helper,
	instance topologyHandler,
	finalizer string,
	conditionUpdater conditionUpdater,
	defaultLabelSelector metav1.LabelSelector,
) (*topologyv1.Topology, error) {
	return internalcommon.EnsureTopology(ctx, h, instance, finalizer, conditionUpdater, defaultLabelSelector)
}

func ensureSecret(
	ctx context.Context,
	secretName types.NamespacedName,
	expectedFields []string,
	reader client.Reader,
	conditionUpdater conditionUpdater,
	requeueTimeout time.Duration,
) (string, ctrl.Result, corev1.Secret, error) {
	return internalcommon.EnsureSecret(
		ctx,
		secretName,
		expectedFields,
		reader,
		conditionUpdater,
		requeueTimeout,
		novav1.InputReadyWaitingMessage,
		"secret/"+secretName.Name,
	)
}

func ensureNetworkAttachments(
	ctx context.Context,
	h *helper.Helper,
	networkAttachments []string,
	conditionUpdater conditionUpdater,
	requeueTimeout time.Duration,
) (map[string]string, ctrl.Result, error) {
	return internalcommon.EnsureNetworkAttachments(
		ctx,
		h,
		networkAttachments,
		h.GetBeforeObject().GetNamespace(),
		conditionUpdater,
		requeueTimeout,
	)
}

func allSubConditionIsTrue(conditionsGetter conditionsGetter) bool {
	return internalcommon.AllSubConditionIsTrue(conditionsGetter)
}

func hashOfStringMap(input map[string]string) (string, error) {
	return internalcommon.HashOfStringMap(input)
}

// OwnedBy returns true if the owner has an OwnerReference on the owned object.
func OwnedBy(owned client.Object, owner client.Object) bool {
	return internalcommon.OwnedBy(owned, owner)
}

func cleanNovaServiceFromNovaDb(
	ctx context.Context,
	computeClient *gophercloud.ServiceClient,
	serviceName string,
	l logr.Logger,
	replicaCount int32,
	cellName string,
) error {
	opts := services.ListOpts{
		Binary: serviceName,
	}

	allPages, err := services.List(computeClient, opts).AllPages(ctx)
	if err != nil {
		return err
	}

	allServices, err := services.ExtractServices(allPages)
	if err != nil {
		return err
	}

	for _, service := range allServices {
		if !strings.Contains(service.Host, cellName) {
			continue
		}

		hostSplits := strings.Split(service.Host, "-")
		hostIndexStr := hostSplits[len(hostSplits)-1]

		hostIndex, err := strconv.Atoi(hostIndexStr)
		if err != nil {
			return err
		}

		if hostIndex >= int(replicaCount) {
			rsp := services.Delete(ctx, computeClient, service.ID)
			if rsp.Err != nil {
				l.Error(rsp.Err, "Failed to delete service", "service", service, "response", rsp)
				return rsp.Err
			}
			l.Info("Deleted service", "service", service)
		}
	}

	return err
}

func (r *ReconcilerBase) ensureMetadataDeleted(
	ctx context.Context,
	instance client.Object,
) error {
	Log := r.GetLogger(ctx)
	metadataName := getNovaMetadataName(instance)
	metadata := &novav1.NovaMetadata{}
	err := r.Client.Get(ctx, metadataName, metadata)
	if k8s_errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !OwnedBy(metadata, instance) {
		Log.Info("NovaMetadata is disabled, but there is a "+
			"NovaMetadata CR not owned by us. Not deleting it.",
			"NovaMetadata", metadata)
		return nil
	}

	err = r.Client.Delete(ctx, metadata)
	if err != nil && k8s_errors.IsNotFound(err) {
		return nil
	}
	Log.Info("NovaMetadata is disabled, so deleted NovaMetadata",
		"NovaMetadata", metadata)

	return nil
}

type clientAuth interface {
	GetKeystoneAuthURL() string
	GetKeystoneUser() string
	GetCABundleSecretName() string
	GetRegion() string
}

func getNovaClient(
	ctx context.Context,
	h *helper.Helper,
	auth clientAuth,
	password string,
	appCredID string,
	appCredSecret string,
	l logr.Logger,
) (*gophercloud.ServiceClient, error) {
	authURL := auth.GetKeystoneAuthURL()
	parsedAuthURL, err := url.Parse(authURL)
	if err != nil {
		return nil, err
	}

	var tlsConfig *openstack.TLSConfig

	if parsedAuthURL.Scheme == "https" {
		caCert, ctrlResult, err := secret.GetDataFromSecret(
			ctx,
			h,
			auth.GetCABundleSecretName(),
			time.Second,
			tls.CABundleKey)
		if err != nil {
			return nil, err
		}
		if (ctrlResult != ctrl.Result{}) {
			err = k8s_errors.NewNotFound(
				appsv1.Resource("Secret"),
				fmt.Sprintf("the CABundleSecret %s not found", auth.GetCABundleSecretName()))
			return nil, err
		}

		tlsConfig = &openstack.TLSConfig{
			CACerts: []string{
				caCert,
			},
		}
	}

	cfg := openstack.AuthOpts{
		AuthURL:                     authURL,
		Username:                    auth.GetKeystoneUser(),
		Password:                    password,
		DomainName:                  "Default",   // fixme
		Region:                      "regionOne", // fixme
		TenantName:                  "service",   // fixme
		TLS:                         tlsConfig,
		ApplicationCredentialID:     appCredID,
		ApplicationCredentialSecret: appCredSecret,
	}
	endpointOpts := gophercloud.EndpointOpts{
		Region:       cfg.Region,
		Availability: gophercloud.AvailabilityInternal,
	}
	computeClient, err := openstack.GetNovaOpenStackClient(ctx, l, cfg, endpointOpts)
	if err != nil {
		return nil, err
	}

	client := computeClient.GetOSClient()
	client.Microversion = "2.95"
	return client, nil
}

func getNovaCellCRName(novaCRName string, cellName string) string {
	return novaCRName + "-" + cellName
}

func getCellDatabaseName(cellName string) string {
	return "nova_" + cellName
}

func getMemcachedInstance(instance *novav1.Nova, cellTemplate novav1.NovaCellTemplate) string {
	if cellTemplate.MemcachedInstance != "" {
		return cellTemplate.MemcachedInstance
	}
	return instance.Spec.MemcachedInstance
}

func ensureMemcached(
	ctx context.Context,
	h *helper.Helper,
	namespaceName string,
	memcachedName string,
	conditionUpdater conditionUpdater,
) (*memcachedv1.Memcached, error) {
	memcached, err := memcachedv1.GetMemcachedByName(ctx, h, memcachedName, namespaceName)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			conditionUpdater.Set(condition.FalseCondition(
				condition.MemcachedReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.MemcachedReadyWaitingMessage))
			return nil, fmt.Errorf("%w: memcached %s not found", err, memcachedName)
		}
		conditionUpdater.Set(condition.FalseCondition(
			condition.MemcachedReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.MemcachedReadyErrorMessage,
			err.Error()))
		return nil, err
	}

	if !memcached.IsReady() {
		conditionUpdater.Set(condition.FalseCondition(
			condition.MemcachedReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.MemcachedReadyWaitingMessage))
		return nil, fmt.Errorf("%w: memcached %s is not ready", util.ErrResourceIsNotReady, memcachedName)
	}
	conditionUpdater.MarkTrue(condition.MemcachedReadyCondition, condition.MemcachedReadyMessage)

	return memcached, err
}

// SortNovaCellListByName sorts a NovaCellList by name in ascending order.
func SortNovaCellListByName(cellList *novav1.NovaCellList) {
	sort.SliceStable(cellList.Items, func(i, j int) bool {
		return cellList.Items[i].Name < cellList.Items[j].Name
	})
}

func parseQuorumQueues(data []byte) bool {
	if data == nil {
		return false
	}
	return string(data) == "true"
}
