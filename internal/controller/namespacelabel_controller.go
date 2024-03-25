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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	multinamespacelabelv1 "my.domain/namespacelabel/api/v1"
	"my.domain/namespacelabel/internal/finalizer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
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

	log.Info(fmt.Sprintf("ensuring Namespace '%s'", req.Namespace))

	if err := r.ensureNamespace(ctx, req.Namespace, namespace); err != nil {
		log.Error(err, "unable to ensure Namespace", "namespace", namespace)
		return ctrl.Result{}, err
	}

	log.Info("get NamespaceLabel list\n")
	namespaceLabelList := &multinamespacelabelv1.NamespaceLabelList{}
	if err := r.List(ctx, namespaceLabelList, client.InNamespace(req.Namespace)); err != nil {
		log.Error(err, fmt.Sprintf("Failed to get NamespaceLabel list %v\n", err))
		return ctrl.Result{}, err
	}

	aggregatedLabels := make(map[string]string)

	err := r.aggregateLabels(ctx, namespaceLabelList, aggregatedLabels)
	if err != nil {
		log.Error(err, fmt.Sprintf("Failed to aggregate NamespaceLabel list labels %v\n", err))
		return ctrl.Result{}, err
	}

	// keeps existing 'app.kubernetes.io/' prefixed labels from the Namespace untouched
	for key, value := range namespace.Labels {
		if strings.HasPrefix(key, multinamespacelabelv1.RecommendedLabelPrefix) {
			log.Info(fmt.Sprintf("Preserving Label '%s' with value '%s'\n", key, value))
			aggregatedLabels[key] = value
		}
	}

	// Update Namespace with aggregated labels
	log.Info(fmt.Sprintf("Updating Namespace '%s' with aggregated Labels\n", namespace.Name))
	namespace.Labels = aggregatedLabels

	if err := r.Update(ctx, namespace); err != nil {
		log.Error(err, fmt.Sprintf("Failed to update Namespace '%s' with aggregated Labels: %v\n", namespace.Name, err))
		return ctrl.Result{}, err
	}

	for _, nsLabel := range namespaceLabelList.Items {
		r.updateNamespaceLabelStatus(ctx, nsLabel)
		err, isDeleted := finalizer.HandleNsLabelDeletion(ctx, nsLabel, namespace, r.Client)
		if err != nil {
			log.Error(err, fmt.Sprintf("Failed to handle NamespaceLabel deletion: %s", err.Error()))
			return ctrl.Result{}, err
		}
		if isDeleted {
			return ctrl.Result{}, nil
		}
		if err := finalizer.EnsureFinalizer(ctx, nsLabel, r.Client); err != nil {
			log.Error(err, fmt.Sprintf("Failed to ensure finalizer in NamespaceLabel: %s", err.Error()))
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// ensureNamespace gets namespace by name and if it does not exist it creates one with the provided name
func (r *NamespaceLabelReconciler) ensureNamespace(ctx context.Context, namespaceName string, namespace *corev1.Namespace) error {
	log := log.FromContext(ctx)

	log.Info(fmt.Sprintf("try get Namespace '%s'", namespaceName))
	if err := r.Get(ctx, client.ObjectKey{Name: namespaceName}, namespace); err != nil {
		log.Error(err, fmt.Sprintf("Failed to get Namespace %s: %v\n", namespaceName, err))
		// If the namespace doesn't exist, create it
		if apierrors.IsNotFound(err) {
			log.Info(fmt.Sprintf("Namespace doesn't exist, Creating Namespace '%s", namespaceName))
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespaceName,
				},
			}

			// Attempt to create the namespace
			if err = r.Create(ctx, namespace); err != nil {
				log.Error(err, fmt.Sprintf("Failed to create Namespace %s: %v\n", namespaceName, err))
				return err
			}
		}
		return err
	}
	return nil
}

// aggregateLabels accumulates namespaceLabels list labels exculding conflicting labels
// If there are labels with same key and different value it shall update the current namespaceLabel status,
// and the first label value shall remain
func (r *NamespaceLabelReconciler) aggregateLabels(ctx context.Context, namespaceLabelList *multinamespacelabelv1.NamespaceLabelList, aggregatedLabels map[string]string) error {
	log := log.FromContext(ctx)

	log.Info("run over NamespaceLabel list\n")
	for _, nsLabel := range namespaceLabelList.Items {
		log.Info("aggregate NamespaceLabel labels\n")
		for key, value := range nsLabel.Spec.Labels {
			if strings.HasPrefix(key, multinamespacelabelv1.RecommendedLabelPrefix) {
				log.Info("skip over app.kubernetes.io/ prefixed labels\n")
				continue // Skip app.kubernetes.io/ prefixed labels
			}
			if existingVal, exists := aggregatedLabels[key]; exists && existingVal != value {
				log.Info(fmt.Sprintf("Conflicting Label '%s' found, update the CRD status\n", key))
				// Conflicting label found, update the CRD status
				condition := metav1.Condition{
					Type:               string(multinamespacelabelv1.SyncStatusFailed),
					Status:             metav1.ConditionTrue,
					Reason:             "LabelConflict",
					Message:            fmt.Sprintf("Label '%s' has conflicting values", key),
					LastTransitionTime: metav1.Now(),
				}
				nsLabel.Status.Conditions = append(nsLabel.Status.Conditions, condition)
				nsLabel.Status.Phase = string(multinamespacelabelv1.SyncStatusFailed)
				if err := r.Status().Update(ctx, &nsLabel); err != nil {
					log.Error(err, fmt.Sprintf("Failed to update NamespaceLabel '%s'- conflict label status %v\n", nsLabel.Name, err))
					return err
				}
				continue
			}
			aggregatedLabels[key] = value
		}
	}
	return nil
}

// updateNamespaceLabelStatus updates namespaceLabel status to success.
func (r *NamespaceLabelReconciler) updateNamespaceLabelStatus(ctx context.Context, nsLabel multinamespacelabelv1.NamespaceLabel) error {
	log := log.FromContext(ctx)
	log.Info("Update CRD status to Success for those without conflicts\n")
	hasConflict := false
	for _, cond := range nsLabel.Status.Conditions {
		if cond.Type == string(multinamespacelabelv1.SyncStatusFailed) && cond.Status == metav1.ConditionTrue {
			hasConflict = true
			break
		}
	}
	if !hasConflict {
		// If the current NamespaceLabel instance has no conflict, update its status
		log.Info(fmt.Sprintf("NamespaceLabel '%s' instance has no conflict updating its status\n", nsLabel.Name))
		nsLabel.Status.Phase = string(multinamespacelabelv1.SyncStatusCompleted)
		nsLabel.Status.Conditions = append(nsLabel.Status.Conditions, metav1.Condition{
			Type:               string(multinamespacelabelv1.SyncStatusCompleted),
			Status:             metav1.ConditionTrue,
			Reason:             "SuccessfulSync",
			Message:            "Successfully synchronized namespace labels",
			LastTransitionTime: metav1.Now(),
		})

		if err := r.Status().Update(ctx, &nsLabel); err != nil {
			// Log the error and continue processing other NamespaceLabel instances
			log.Error(err, fmt.Sprintf("Failed to update NamespaceLabel status for %s: %v\n", nsLabel.Name, err))
			return err
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NamespaceLabelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&multinamespacelabelv1.NamespaceLabel{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}
