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
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestKustomizationReconciler_reconcileDelete_OrphansWhenKubeConfigSecretMissing(t *testing.T) {
	scheme := runtime.NewScheme()

	_ = corev1.AddToScheme(scheme)
	_ = kustomizev1.AddToScheme(scheme)

	reconciler := KustomizationReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
	}

	kustomization := kustomizev1.Kustomization{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-kustomization",
			Namespace: "testing",
		},
		Spec: kustomizev1.KustomizationSpec{
			Prune: true,
			Interval: metav1.Duration{
				Duration: 5 * time.Minute,
			},
			KubeConfig: &meta.KubeConfigReference{
				SecretRef: meta.SecretKeyReference{
					Name: "missing-kubeconfig",
				},
			},
		},
	}

	_, err := reconciler.reconcileDelete(context.Background(), &kustomization)
	if err != nil {
		t.Fatal(err)
	}

	if kustomization.Spec.Prune {
		t.Fatalf("expected prune=false, got true")
	}

	if kustomization.Spec.Interval.Duration != time.Minute {
		t.Fatalf("expected interval=%s, got %s", time.Minute, kustomization.Spec.Interval.Duration)
	}
}

func TestKustomizationReconciler_reconcileDelete_DoesNotChangeWhenKubeConfigSecretExists(t *testing.T) {
	scheme := runtime.NewScheme()

	_ = corev1.AddToScheme(scheme)
	_ = kustomizev1.AddToScheme(scheme)

	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "present-kubeconfig",
			Namespace: "testing",
		},
	}

	reconciler := KustomizationReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(&secret).Build(),
	}

	kustomization := kustomizev1.Kustomization{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-kustomization",
			Namespace: "testing",
		},
		Spec: kustomizev1.KustomizationSpec{
			Prune: true,
			Interval: metav1.Duration{
				Duration: 5 * time.Minute,
			},
			KubeConfig: &meta.KubeConfigReference{
				SecretRef: meta.SecretKeyReference{
					Name: secret.Name,
				},
			},
		},
	}

	_, err := reconciler.reconcileDelete(context.Background(), &kustomization)
	if err != nil {
		t.Fatal(err)
	}

	if !kustomization.Spec.Prune {
		t.Fatalf("expected prune=true, got false")
	}

	if kustomization.Spec.Interval.Duration != 5*time.Minute {
		t.Fatalf("expected interval=%s, got %s", 5*time.Minute, kustomization.Spec.Interval.Duration)
	}
}
