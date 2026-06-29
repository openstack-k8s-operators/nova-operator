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
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	memcachedv1 "github.com/openstack-k8s-operators/infra-operator/apis/memcached/v1beta1"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/nova/v1beta1"
	internalcommon "github.com/openstack-k8s-operators/nova-operator/internal/common"

	gophercloud "github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/services"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	util "github.com/openstack-k8s-operators/lib-common/modules/common/util"
	"github.com/openstack-k8s-operators/lib-common/modules/openstack"
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
)

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
		// delete only if serviceHost is for our cell. If no cell is
		// provided (e.g. scheduler cleanup case) then this check is
		// non-operational (noop)
		if !strings.Contains(service.Host, cellName) {
			continue
		}

		// extract 0 (suffix) from hostname nova-scheduler-0
		hostSplits := strings.Split(service.Host, "-")
		hostIndexStr := hostSplits[len(hostSplits)-1]

		hostIndex, err := strconv.Atoi(hostIndexStr)
		if err != nil {
			return err
		}

		// name index start from 0
		// which means if replicaCount is 1, only nova-scheduler-0 is valid case
		// so delete >= 1
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

// ReconcilerBase provides a common set of clients scheme and loggers for all reconcilers.
type ReconcilerBase struct {
	internalcommon.ReconcilerBase
}

// NewReconcilerBase constructs a ReconcilerBase given a manager and Kclient.
func NewReconcilerBase(
	mgr ctrl.Manager, kclient kubernetes.Interface,
) ReconcilerBase {
	return ReconcilerBase{
		ReconcilerBase: internalcommon.NewReconcilerBase(mgr, kclient),
	}
}

// NewReconcilers constructs all nova Reconciler objects
func NewReconcilers(mgr ctrl.Manager, kclient *kubernetes.Clientset) *internalcommon.Reconcilers {
	return internalcommon.NewReconcilers(map[string]internalcommon.Reconciler{
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

// novaAdditionalTemplates returns the default extra config templates for nova services.
func novaAdditionalTemplates() map[string]string {
	return map[string]string{
		"01-nova.conf":    "/nova/nova.conf",
		"nova-blank.conf": "/nova/nova-blank.conf",
	}
}

func getNovaCellCRName(novaCRName string, cellName string) string {
	return novaCRName + "-" + cellName
}

func hashOfStringMap(input map[string]string) (string, error) {
	// we need to make sure that we have a stable map iteration order and
	// hash both the keys and the values
	keys := make([]string, 0, len(input))
	for k := range input {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	keyValues := []string{}
	for _, key := range keys {
		value := input[key]
		keyValues = append(keyValues, key+value)
	}
	return util.ObjectHash(keyValues)
}

// GetSecret defines an interface for objects that can provide a secret name
type GetSecret interface {
	GetSecret() string
	client.Object
}

// GetLogger returns a logger object with a prefix of "controller.name" and additional controller context fields
func (r *ReconcilerBase) GetLogger(ctx context.Context) logr.Logger {
	return log.FromContext(ctx).WithName("Controllers").WithName("ReconcilerBase")
}

// OwnedBy returns true if the owner has an OwnerReference on the owned object
func OwnedBy(owned client.Object, owner client.Object) bool {
	for _, ref := range owned.GetOwnerReferences() {
		if owner.GetUID() == ref.UID {
			return true
		}
	}
	return false
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
		// Nothing to do as it does not exists
		return nil
	}
	if err != nil {
		return err
	}
	// If it is not created by us, we don't touch it
	if !OwnedBy(metadata, instance) {
		Log.Info("NovaMetadata is disabled, but there is a "+
			"NovaMetadata CR not owned by us. Not deleting it.",
			"NovaMetadata", metadata)
		return nil
	}

	// OK this was created by us so we go and delete it
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
			// requeue is translated to error below as the secret already
			// verified to exists and has the expected fields.
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
	// NOTE(gibi): We use Antelope maximum because we can. In reality we only
	// need 2.53 to be able to delete services based on UUID.
	client.Microversion = "2.95"
	return client, nil
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

// ensureMemcached - gets the Memcached instance cell specific used for nova services cache backend
func ensureMemcached(
	ctx context.Context,
	h *helper.Helper,
	namespaceName string,
	memcachedName string,
	conditionUpdater internalcommon.ConditionUpdater,
) (*memcachedv1.Memcached, error) {
	memcached, err := memcachedv1.GetMemcachedByName(ctx, h, memcachedName, namespaceName)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Memcached should be automatically created by the encompassing OpenStackControlPlane,
			// but we don't propagate its name into the "memcachedInstance" field of other sub-resources,
			// so if it is missing at this point, it *could* be because there's a mismatch between the
			// name of the Memcached CR and the name of the Memcached instance referenced by this CR.
			// Since that situation would block further reconciliation, we treat it as a warning.
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

// SortNovaCellListByName sorts a NovaCellList by name in ascending order
func SortNovaCellListByName(cellList *novav1.NovaCellList) {
	sort.SliceStable(cellList.Items, func(i, j int) bool {
		return cellList.Items[i].Name < cellList.Items[j].Name
	})
}

// parseQuorumQueues parses the quorum queues value from secret data
func parseQuorumQueues(data []byte) bool {
	if data == nil {
		return false
	}
	return string(data) == "true"
}
