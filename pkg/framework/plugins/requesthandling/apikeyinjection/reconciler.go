/*
Copyright 2026 The llm-d Authors.

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

package apikeyinjection

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	logutil "github.com/llm-d/llm-d-inference-payload-processor/pkg/common/observability/logging"
)

const ippManagedLabel = "inference.llm-d.ai/ipp-managed"

func hasIPPManagedLabel(object client.Object) bool {
	return object.GetLabels()[ippManagedLabel] == "true"
}

// ippManagedPredicate filters events to only Secrets labeled with
// "inference.llm-d.ai/ipp-managed" = "true".
func ippManagedPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool { return hasIPPManagedLabel(e.Object) },
		UpdateFunc: func(e event.UpdateEvent) bool {
			return hasIPPManagedLabel(e.ObjectOld) || hasIPPManagedLabel(e.ObjectNew)
		},
		DeleteFunc:  func(e event.DeleteEvent) bool { return hasIPPManagedLabel(e.Object) },
		GenericFunc: func(e event.GenericEvent) bool { return hasIPPManagedLabel(e.Object) },
	}
}

// secretReconciler watches Secrets and updates the secretStore.
type secretReconciler struct {
	client.Reader
	store *secretStore
}

func (r *secretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).V(logutil.DEFAULT)
	key := req.String()
	logger.Info("reconciling Secret", "key", key)

	secret := &corev1.Secret{}
	err := r.Get(ctx, req.NamespacedName, secret)
	if err != nil && !errors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("unable to get Secret: %w", err)
	}

	if errors.IsNotFound(err) || !secret.DeletionTimestamp.IsZero() || !hasIPPManagedLabel(secret) {
		r.store.delete(req.Namespace, req.Name)
		logger.Info("Secret removed from store", "key", key)
		return ctrl.Result{}, nil
	}

	if err := r.store.addOrUpdate(req.Namespace, req.Name, secret); err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to add or update Secret %s: %w", key, err)
	}

	logger.Info("Secret added/updated in store", "key", key)
	return ctrl.Result{}, nil
}
