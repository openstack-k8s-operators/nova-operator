/*
Copyright 2026.

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

package common //nolint:revive // common is the established package name for multi-group shared code

import (
	"context"
	"fmt"
	"time"

	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	nad "github.com/openstack-k8s-operators/lib-common/modules/common/networkattachment"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// EnsureNetworkAttachments checks requested network attachments exist and returns pod annotations.
func EnsureNetworkAttachments(
	ctx context.Context,
	h *helper.Helper,
	networkAttachments []string,
	namespace string,
	conditionUpdater ConditionUpdater,
	requeueTimeout time.Duration,
) (map[string]string, ctrl.Result, error) {
	var nadAnnotations map[string]string

	nadList := []networkv1.NetworkAttachmentDefinition{}
	for _, netAtt := range networkAttachments {
		netDef, err := nad.GetNADWithName(ctx, h, netAtt, namespace)
		if err != nil {
			if k8s_errors.IsNotFound(err) {
				log.FromContext(ctx).Info(fmt.Sprintf("network-attachment-definition %s not found", netAtt))
				conditionUpdater.Set(condition.FalseCondition(
					condition.NetworkAttachmentsReadyCondition,
					condition.ErrorReason,
					condition.SeverityWarning,
					condition.NetworkAttachmentsReadyWaitingMessage,
					netAtt))
				return nadAnnotations, ctrl.Result{RequeueAfter: requeueTimeout}, nil
			}
			conditionUpdater.Set(condition.FalseCondition(
				condition.NetworkAttachmentsReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.NetworkAttachmentsErrorMessage,
				err.Error()))
			return nadAnnotations, ctrl.Result{}, err
		}

		if netDef != nil {
			nadList = append(nadList, *netDef)
		}
	}

	var err error
	nadAnnotations, err = nad.EnsureNetworksAnnotation(nadList)
	if err != nil {
		return nadAnnotations, ctrl.Result{}, fmt.Errorf("failed create network annotation from %s: %w",
			networkAttachments, err)
	}

	return nadAnnotations, ctrl.Result{}, nil
}
