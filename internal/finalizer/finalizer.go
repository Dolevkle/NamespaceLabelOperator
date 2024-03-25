package finalizer

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	multinamespacelabelv1 "my.domain/namespacelabel/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const FinalizerCleanupCapp = "multinamespacelabel/cleanup"

// HandleResourceDeletion is responsible for handeling namespaceLabel deletion
func HandleNsLabelDeletion(ctx context.Context, nsLabel multinamespacelabelv1.NamespaceLabel, namespace *corev1.Namespace, r client.Client) (error, bool) {
	if nsLabel.ObjectMeta.DeletionTimestamp != nil {
		if controllerutil.ContainsFinalizer(&nsLabel, FinalizerCleanupCapp) {
			if err := finalizeNsLabel(ctx, nsLabel, namespace, r); err != nil {
				return err, false
			}
			return RemoveFinalizer(ctx, nsLabel, r), true
		}
	}
	return nil, false
}

// RemoveFinalizer removes the finalizer from the namespaceLabel
func RemoveFinalizer(ctx context.Context, nsLabel multinamespacelabelv1.NamespaceLabel, r client.Client) error {
	controllerutil.RemoveFinalizer(&nsLabel, FinalizerCleanupCapp)
	if err := r.Update(ctx, &nsLabel); err != nil {
		return err
	}
	return nil
}

// finalizeNsLabel runs the cleanup of all the labels of the namespaceLabel from namespace
func finalizeNsLabel(ctx context.Context, nsLabel multinamespacelabelv1.NamespaceLabel, namespace *corev1.Namespace, r client.Client) error {
	// Remove labels that are specified in the NamespaceLabel CRD from the namespace
	modified := false
	for key := range nsLabel.Spec.Labels {
		if _, exists := namespace.Labels[key]; exists {
			delete(namespace.Labels, key)
			modified = true
		}
	}

	// Update the namespace if any labels were removed
	if modified {
		if err := r.Update(ctx, namespace); err != nil {
			return err
		}
	}
	return nil
}

// EnsureFinalizer ensures the namespace label has the finalizer.
func EnsureFinalizer(ctx context.Context, nsLabel multinamespacelabelv1.NamespaceLabel, r client.Client) error {
	if !controllerutil.ContainsFinalizer(&nsLabel, FinalizerCleanupCapp) {
		controllerutil.AddFinalizer(&nsLabel, FinalizerCleanupCapp)
		if err := r.Update(ctx, &nsLabel); err != nil {
			return err
		}
	}
	return nil
}
