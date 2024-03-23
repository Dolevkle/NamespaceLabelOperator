/*
Copyright 2024.

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

package controller

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	multinamespacelabelv1 "my.domain/namespacelabel/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// NamespaceLabelReconciler reconciles a NamespaceLabel object
type NamespaceLabelReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=multinamespacelabel.my.domain,resources=namespacelabels,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=multinamespacelabel.my.domain,resources=namespacelabels/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=multinamespacelabel.my.domain,resources=namespacelabels/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NamespaceLabel object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.0/pkg/reconcile
func (r *NamespaceLabelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	namespace := &corev1.Namespace{}

	log.Info("get Namespace-", req.Namespace)
	if err := r.Get(ctx, client.ObjectKey{Name: req.Namespace}, namespace); err != nil {
		log.Info("Failed to get Namespace")
		return ctrl.Result{}, err
	}

	log.Info("get NamespaceLabel list")
	namespaceLabelList := &multinamespacelabelv1.NamespaceLabelList{}
	if err := r.List(ctx, namespaceLabelList, client.InNamespace(req.Namespace)); err != nil {
		log.Info("Failed to get NamespaceLabel list")
		return ctrl.Result{}, err
	}

	aggregatedLabels := map[string]string{}
	log.Info("run over NamespaceLabel list")
	for _, nsLabel := range namespaceLabelList.Items {
		for key, value := range nsLabel.Spec.Labels {
			log.Info("aggregate NamespaceLabel labels")
			if strings.HasPrefix(key, multinamespacelabelv1.RecommendedLabelPrefix) {
				log.Info("skip over app.kubernetes.io/ prefixed labels")
				continue // Skip app.kubernetes.io/ prefixed labels
			}
			if existingVal, exists := aggregatedLabels[key]; exists && existingVal != value {
				log.Info(fmt.Sprintf("Conflicting Label '%s' found, update the CRD status", key))
				// Conflicting label found, update the CRD status
				condition := metav1.Condition{
					Type:    string(multinamespacelabelv1.SyncStatusFailed),
					Status:  metav1.ConditionTrue,
					Reason:  "LabelConflict same key different value",
					Message: fmt.Sprintf("Label '%s' has conflicting values", key),
				}
				nsLabel.Status.Conditions = append(nsLabel.Status.Conditions, condition)
				nsLabel.Status.Phase = string(multinamespacelabelv1.SyncStatusFailed)
				if err := r.Status().Update(ctx, &nsLabel); err != nil {
					log.Info("Failed to update crd conflict label status")
					return ctrl.Result{}, err
				}
				continue
			}
			aggregatedLabels[key] = value
		}
	}

	// Preserve existing 'app.kubernetes.io/' prefixed labels from the Namespace
	for key, value := range namespace.Labels {
		if strings.HasPrefix(key, multinamespacelabelv1.RecommendedLabelPrefix) {
			log.Info(fmt.Sprintf("Preserving Label '%s' with value '%s", key, value))
			aggregatedLabels[key] = value
		}
	}
	// Update Namespace with aggregated labels
	log.Info(fmt.Sprintf("Updating Namespace '%s' with aggregated Labels", namespace.Name))
	namespace.Labels = aggregatedLabels

	if err := r.Update(ctx, namespace); err != nil {
		log.Info(fmt.Sprintf("Failed to update Namespace '%s' with aggregated Labels", namespace.Name))
		return ctrl.Result{}, err
	}

	// Update CRD status to Success for those without conflicts
	log.Info("Update CRD status to Success for those without conflicts")
	for _, nsLabel := range namespaceLabelList.Items {
		hasConflict := false
		for _, cond := range nsLabel.Status.Conditions {
			if cond.Type == string(multinamespacelabelv1.SyncStatusFailed) && cond.Status == metav1.ConditionTrue {
				hasConflict = true
				break
			}
		}
		if !hasConflict {
			// If the current NamespaceLabel instance has no conflict, update its status
			log.Info(fmt.Sprintf("NamespaceLabel '%s' instance has no conflict updating its status", nsLabel.Name))
			nsLabel.Status.Phase = string(multinamespacelabelv1.SyncStatusCompleted)
			nsLabel.Status.Conditions = append(nsLabel.Status.Conditions, metav1.Condition{
				Type:    string(multinamespacelabelv1.SyncStatusCompleted),
				Status:  metav1.ConditionTrue,
				Reason:  "SuccessfulSync",
				Message: "Successfully synchronized namespace labels",
			})

			if err := r.Status().Update(ctx, &nsLabel); err != nil {
				// Log the error and continue processing other NamespaceLabel instances
				log.Info(fmt.Sprintf("Failed to update NamespaceLabel status for %s: %v\n", nsLabel.Name, err))
			}
		}

		// TODO move to a function
		const namespaceLabelFinalizer = "namespacelabel.finalizers.yourdomain.com"
		// Handle finalizer logic
		if nsLabel.ObjectMeta.DeletionTimestamp.IsZero() {
			// The object is not being deleted, so if it does not have our finalizer,
			// then lets add the finalizer and update the object.
			if !containsString(nsLabel.ObjectMeta.Finalizers, namespaceLabelFinalizer) {
				nsLabel.ObjectMeta.Finalizers = append(nsLabel.ObjectMeta.Finalizers, namespaceLabelFinalizer)
				if err := r.Update(ctx, &nsLabel); err != nil {
					return ctrl.Result{}, err
				}
			}
		} else {
			// The object is being deleted
			if containsString(nsLabel.ObjectMeta.Finalizers, namespaceLabelFinalizer) {
				// our finalizer is present, so lets handle any external dependency

				// Remove our finalizer from the list and update it.
				nsLabel.ObjectMeta.Finalizers = removeString(nsLabel.ObjectMeta.Finalizers, namespaceLabelFinalizer)
				if err := r.Update(ctx, &nsLabel); err != nil {
					return ctrl.Result{}, err
				}
			}

			// Stop reconciliation as the item is being deleted
			return ctrl.Result{}, nil
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NamespaceLabelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&multinamespacelabelv1.NamespaceLabel{}).
		Complete(r)
}

// Helper functions to check and remove string from a slice of strings.
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func removeString(slice []string, s string) []string {
	result := []string{}
	for _, item := range slice {
		if item != s {
			result = append(result, item)
		}
	}
	return result
}
