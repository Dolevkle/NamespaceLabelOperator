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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	multinamespacelabelv1 "my.domain/namespacelabel/api/v1"
	"my.domain/namespacelabel/internal/finalizer"
)

var _ = Describe("NamespaceLabel Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-ns-label"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "test-ns",
		}
		namespacelabel := &multinamespacelabelv1.NamespaceLabel{}
		namespace := &corev1.Namespace{}

		BeforeEach(func() {
			By("creating the namespace for the Kind NamespaceLabel")
			err := k8sClient.Get(ctx, client.ObjectKey{Name: typeNamespacedName.Namespace}, namespace)
			if err != nil && errors.IsNotFound(err) {
				namespace = &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: typeNamespacedName.Namespace,
					},
				}
				Expect(k8sClient.Create(ctx, namespace)).To(Succeed())
			}
			By("creating the custom resource for the Kind NamespaceLabel")
			err = k8sClient.Get(ctx, typeNamespacedName, namespacelabel)
			if err != nil && errors.IsNotFound(err) {
				resource := &multinamespacelabelv1.NamespaceLabel{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: typeNamespacedName.Namespace,
						Finalizers: []string{
							finalizer.FinalizerCleanupNsLabel,
						},
					},
					// TODO(user): Specify other spec details if needed.
					Spec: multinamespacelabelv1.NamespaceLabelSpec{
						Labels: map[string]string{
							"hello":   "world",
							"welcome": "roni",
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}

		})

		AfterEach(func() {
			resource := &multinamespacelabelv1.NamespaceLabel{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance NamespaceLabel")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &NamespaceLabelReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})

			Expect(err).NotTo(HaveOccurred())
			err = k8sClient.Get(ctx, client.ObjectKey{Name: typeNamespacedName.Namespace}, namespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to fetch namespace: %s", typeNamespacedName.Name)
			for key, expectedValue := range namespacelabel.Spec.Labels {
				actualValue, exists := namespace.Labels[key]
				Expect(exists).To(BeTrue(), "Label %s does not exist in namespace %s", key, typeNamespacedName.Name)
				Expect(actualValue).To(Equal(expectedValue), "Label %s in namespace %s does not match expected value", key, typeNamespacedName.Name)
			}
		})

		It("should show conflict label when reconciling the resource", func() {
			resource := &multinamespacelabelv1.NamespaceLabel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ns-label2",
					Namespace: typeNamespacedName.Namespace,
					Finalizers: []string{
						finalizer.FinalizerCleanupNsLabel,
					},
				},
				Spec: multinamespacelabelv1.NamespaceLabelSpec{
					Labels: map[string]string{
						"hello":   "world",
						"welcome": "1",
						"sir":     "menny",
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("Reconciling the created resource")
			controllerReconciler := &NamespaceLabelReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})

			Expect(namespace.Labels).To(HaveKey("welcome"))
			Expect(namespace.Labels).ToNot(HaveKeyWithValue("welcome", "1"))
			for key, expectedValue := range namespacelabel.Spec.Labels {
				actualValue, exists := namespace.Labels[key]
				Expect(exists).To(BeTrue(), "Label %s does not exist in namespace %s", key, typeNamespacedName.Name)
				Expect(actualValue).To(Equal(expectedValue), "Label %s in namespace %s does not match expected value", key, typeNamespacedName.Name)
			}
			By("Cleanup the conflict resource instance NamespaceLabel")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())

		})

		It("should show delete labels from ns after reconciling the resource when CRD deleted", func() {
			resource := &multinamespacelabelv1.NamespaceLabel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ns-label3",
					Namespace: typeNamespacedName.Namespace,
					Finalizers: []string{
						finalizer.FinalizerCleanupNsLabel,
					},
				},
				Spec: multinamespacelabelv1.NamespaceLabelSpec{
					Labels: map[string]string{
						"hello":   "world",
						"welcome": "1",
						"there":   "lord",
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("Reconciling the created resource")
			controllerReconciler := &NamespaceLabelReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			By("Cleanup the conflict resource instance NamespaceLabel")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			Expect(namespace.Labels).ToNot(HaveKeyWithValue("there", "lord"))

		})
	})
})
