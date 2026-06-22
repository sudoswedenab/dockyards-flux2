// Copyright 2025 Sudo Sweden AB
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controllers

import (
	"context"
	"testing"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"github.com/google/go-cmp/cmp"
	dockyardsv1 "github.com/sudoswedenab/dockyards-backend/api/v1alpha3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSourceRepositoryReconciler_Reconcile(t *testing.T) {
	tt := []struct {
		name           string
		repository     client.Object
		suffix         string
		expectedKind   string
		expectedLabels map[string]string
	}{
		{
			name: "test helm repository",
			repository: &sourcev1.HelmRepository{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-helm-repository",
					Namespace: "testing",
					UID:       "test-helm-repository-uid",
					Labels: map[string]string{
						dockyardsv1.LabelClusterName:  "test-cluster",
						dockyardsv1.LabelWorkloadName: "test-workload",
					},
				},
			},
			suffix:       HelmRepositorySuffix,
			expectedKind: sourcev1.HelmRepositoryKind,
			expectedLabels: map[string]string{
				dockyardsv1.LabelClusterName:  "test-cluster",
				dockyardsv1.LabelWorkloadName: "test-workload",
			},
		},
		{
			name: "test oci repository",
			repository: &sourcev1.OCIRepository{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-oci-repository",
					Namespace: "testing",
					UID:       "test-oci-repository-uid",
					Labels: map[string]string{
						dockyardsv1.LabelClusterName:  "test-cluster",
						dockyardsv1.LabelWorkloadName: "test-workload",
					},
				},
			},
			suffix:       OCIRepositorySuffix,
			expectedKind: sourcev1.OCIRepositoryKind,
			expectedLabels: map[string]string{
				dockyardsv1.LabelClusterName:  "test-cluster",
				dockyardsv1.LabelWorkloadName: "test-workload",
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			scheme := runtime.NewScheme()

			_ = dockyardsv1.AddToScheme(scheme)
			_ = sourcev1.AddToScheme(scheme)

			c := fake.
				NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tc.repository).
				Build()

			reconciler := SourceRepositoryReconciler{
				Client: c,
			}

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      tc.repository.GetName(),
					Namespace: tc.repository.GetNamespace(),
				},
			}

			_, err := reconciler.Reconcile(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}

			inventoryName := tc.repository.GetName() + tc.suffix

			var workloadInventory dockyardsv1.WorkloadInventory
			err = c.Get(context.Background(), client.ObjectKey{Name: inventoryName, Namespace: tc.repository.GetNamespace()}, &workloadInventory)
			if err != nil {
				t.Fatal(err)
			}

			if len(workloadInventory.OwnerReferences) != 1 {
				t.Fatalf("expected 1 owner reference, got %d", len(workloadInventory.OwnerReferences))
			}

			expectedOwnerReference := metav1.OwnerReference{
				APIVersion: sourcev1.GroupVersion.String(),
				Kind:       tc.expectedKind,
				Name:       tc.repository.GetName(),
				UID:        tc.repository.GetUID(),
			}

			if !cmp.Equal(workloadInventory.OwnerReferences[0], expectedOwnerReference) {
				t.Errorf("diff: %s", cmp.Diff(expectedOwnerReference, workloadInventory.OwnerReferences[0]))
			}

			if !cmp.Equal(workloadInventory.Labels, tc.expectedLabels) {
				t.Errorf("diff: %s", cmp.Diff(tc.expectedLabels, workloadInventory.Labels))
			}

			expectedSelectorLabels := map[string]string{
				"source.toolkit.fluxcd.io/name":      tc.repository.GetName(),
				"source.toolkit.fluxcd.io/namespace": tc.repository.GetNamespace(),
			}

			if !cmp.Equal(workloadInventory.Spec.Selector.MatchLabels, expectedSelectorLabels) {
				t.Errorf("diff: %s", cmp.Diff(expectedSelectorLabels, workloadInventory.Spec.Selector.MatchLabels))
			}
		})
	}
}

func TestSourceRepositoryReconciler_Reconcile_IgnoresRepositoriesWithoutLabels(t *testing.T) {
	tt := []struct {
		name       string
		repository client.Object
		suffix     string
	}{
		{
			name: "test helm repository without labels",
			repository: &sourcev1.HelmRepository{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-helm-repository-without-labels",
					Namespace: "testing",
				},
			},
			suffix: HelmRepositorySuffix,
		},
		{
			name: "test oci repository without labels",
			repository: &sourcev1.OCIRepository{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-oci-repository-without-labels",
					Namespace: "testing",
				},
			},
			suffix: OCIRepositorySuffix,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			scheme := runtime.NewScheme()

			_ = dockyardsv1.AddToScheme(scheme)
			_ = sourcev1.AddToScheme(scheme)

			c := fake.
				NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tc.repository).
				Build()

			reconciler := SourceRepositoryReconciler{
				Client: c,
			}

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      tc.repository.GetName(),
					Namespace: tc.repository.GetNamespace(),
				},
			}

			_, err := reconciler.Reconcile(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}

			var workloadInventory dockyardsv1.WorkloadInventory
			err = c.Get(context.Background(), client.ObjectKey{Name: tc.repository.GetName() + tc.suffix, Namespace: tc.repository.GetNamespace()}, &workloadInventory)
			if !apierrors.IsNotFound(err) {
				t.Fatalf("expected not found, got %v", err)
			}
		})
	}
}
