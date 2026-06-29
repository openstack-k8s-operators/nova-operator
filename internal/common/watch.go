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

	"github.com/go-logr/logr"
	"github.com/openstack-k8s-operators/lib-common/modules/common"
	util "github.com/openstack-k8s-operators/lib-common/modules/common/util"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// FindObjectsForSrcByField returns reconcile requests for CRs in src's namespace
// that reference src via one of the indexed watchFields.
func FindObjectsForSrcByField[L client.ObjectList, E any, S ~[]E](
	ctx context.Context,
	log logr.Logger,
	reader client.Reader,
	src client.Object,
	watchFields []string,
	newList func() L,
	getItems func(L) S,
) []reconcile.Request {
	requests := []reconcile.Request{}

	for _, field := range watchFields {
		crList := newList()
		listOps := &client.ListOptions{
			FieldSelector: fields.OneTermEqualSelector(field, src.GetName()),
			Namespace:     src.GetNamespace(),
		}
		err := reader.List(ctx, crList, listOps)
		if err != nil {
			log.Error(err, fmt.Sprintf("listing %s for field: %s - %s", crList.GetObjectKind().GroupVersionKind().Kind, field, src.GetNamespace()))
			return requests
		}

		requests = appendRequestsForObjects(log, src, requests, itemsAsObjects(getItems(crList)))
	}

	return requests
}

// FindObjectsForSrcInNamespace returns reconcile requests for all CRs in src's namespace.
func FindObjectsForSrcInNamespace[L client.ObjectList, E any, S ~[]E](
	ctx context.Context,
	log logr.Logger,
	reader client.Reader,
	src client.Object,
	newList func() L,
	getItems func(L) S,
) []reconcile.Request {
	requests := []reconcile.Request{}

	crList := newList()
	listOps := &client.ListOptions{
		Namespace: src.GetNamespace(),
	}
	err := reader.List(ctx, crList, listOps)
	if err != nil {
		log.Error(err, fmt.Sprintf("listing %s for namespace: %s", crList.GetObjectKind().GroupVersionKind().Kind, src.GetNamespace()))
		return requests
	}

	return appendRequestsForObjects(log, src, requests, itemsAsObjects(getItems(crList)))
}

// FindObjectsWithAppSelectorLabelInNamespace returns reconcile requests for all CRs
// in src's namespace when src carries an AppSelector label matching allowedServices.
func FindObjectsWithAppSelectorLabelInNamespace[L client.ObjectList, E any, S ~[]E](
	ctx context.Context,
	log logr.Logger,
	reader client.Reader,
	src client.Object,
	allowedServices []string,
	newList func() L,
	getItems func(L) S,
) []reconcile.Request {
	// if the endpoint has the service label and its in our endpointList, reconcile the CR in the namespace
	if svc, ok := src.GetLabels()[common.AppSelector]; ok && util.StringInSlice(svc, allowedServices) {
		return FindObjectsForSrcInNamespace(ctx, log, reader, src, newList, getItems)
	}

	return []reconcile.Request{}
}

func itemsAsObjects[E any, S ~[]E](items S) []client.Object {
	objs := make([]client.Object, len(items))
	for i := range items {
		objs[i] = any(&items[i]).(client.Object)
	}
	return objs
}

func appendRequestsForObjects(
	log logr.Logger,
	src client.Object,
	requests []reconcile.Request,
	items []client.Object,
) []reconcile.Request {
	for _, item := range items {
		log.Info(fmt.Sprintf("input source %s changed, reconcile: %s - %s", src.GetName(), item.GetName(), item.GetNamespace()))

		requests = append(requests,
			reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      item.GetName(),
					Namespace: item.GetNamespace(),
				},
			},
		)
	}

	return requests
}
