package finalizer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/scale/scheme"
	multinamespacelabelv1 "my.domain/namespacelabel/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	_ = multinamespacelabelv1.AddToScheme(s)
	_ = scheme.AddToScheme(s)
	return s
}

func newFakeClient() client.Client {
	scheme := newScheme()
	return fake.NewClientBuilder().WithScheme(scheme).Build()
}

func TestEnsureFinalizer(t *testing.T) {
	ctx := context.Background()
	nsLabel := &multinamespacelabelv1.NamespaceLabel{
		Spec: multinamespacelabelv1.NamespaceLabelSpec{
			Labels: map[string]string{
				"hello":   "world",
				"welcome": "roni",
			},
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-nsLabel",
			Namespace: "test-ns",
		},
	}
	fakeClient := newFakeClient()
	assert.NoError(t, fakeClient.Create(ctx, nsLabel), "Expected no error when creating nsLabel")
	assert.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: "test-nsLabel", Namespace: "test-ns"}, nsLabel))
	assert.NoError(t, EnsureFinalizer(ctx, *nsLabel, fakeClient))
	assert.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: "test-nsLabel", Namespace: "test-ns"}, nsLabel))
	assert.Contains(t, nsLabel.Finalizers, FinalizerCleanupCapp)

	// Check if there is no error after the finalizer exists.
	assert.NoError(t, EnsureFinalizer(ctx, *nsLabel, fakeClient))
}

func TestRemoveFinalizer(t *testing.T) {
	ctx := context.Background()
	nsLabel := &multinamespacelabelv1.NamespaceLabel{
		Spec: multinamespacelabelv1.NamespaceLabelSpec{
			Labels: map[string]string{
				"hello":   "world",
				"welcome": "roni",
			},
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-nsLabel",
			Namespace: "test-ns",
			Finalizers: []string{
				FinalizerCleanupCapp,
			},
		},
	}
	fakeClient := newFakeClient()
	assert.NoError(t, fakeClient.Create(ctx, nsLabel))
	assert.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: "test-nsLabel", Namespace: "test-ns"}, nsLabel))
	assert.NoError(t, RemoveFinalizer(ctx, *nsLabel, fakeClient), "Expected no error when removing finalizer")
	assert.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: "test-nsLabel", Namespace: "test-ns"}, nsLabel))
	assert.NotContains(t, nsLabel.Finalizers, FinalizerCleanupCapp)

	// Check if there is no error after the finalizer removed.
	assert.NoError(t, RemoveFinalizer(ctx, *nsLabel, fakeClient))
}
