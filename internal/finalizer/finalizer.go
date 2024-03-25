package finalizer

import (
	"context"

	multinamespacelabelv1 "my.domain/namespacelabel/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const FinalizerCleanupCapp = "multinamespacelabel/cleanup"

// HandleResourceDeletion is responsible for handeling resource deletion
func HandleNsLabelDeletion(ctx context.Context, nsLabel multinamespacelabelv1.NamespaceLabel, r client.Client) (error, bool) {
	if nsLabel.ObjectMeta.DeletionTimestamp != nil {
		if controllerutil.ContainsFinalizer(&nsLabel, FinalizerCleanupCapp) {
			//TODO refactor
			// if err := finalizeService(capp, resourceManagers); err != nil {
			// 	return err, false
			// }
			return RemoveFinalizer(ctx, nsLabel, r), true
		}
	}
	return nil, false
}

// RemoveFinalizer removes the finalizer from the from the yaml
func RemoveFinalizer(ctx context.Context, nsLabel multinamespacelabelv1.NamespaceLabel, r client.Client) error {
	controllerutil.RemoveFinalizer(&nsLabel, FinalizerCleanupCapp)
	if err := r.Update(ctx, &nsLabel); err != nil {
		return err
	}
	return nil
}

// // fnializeService runs the cleanup of all the resource mangers.
// func finalizeService(capp rcsv1alpha1.Capp, resourceManagers map[string] multinamespacelabelv1.ResourceManager) error {
// 	for _, manager := range resourceManagers {
// 		if err := manager.CleanUp(capp); err != nil {
// 			return err
// 		}
// 	}
// 	return nil
// }

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
