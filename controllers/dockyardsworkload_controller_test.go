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

package controllers_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	"github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	dockyardsv1 "github.com/sudoswedenab/dockyards-backend/api/v1alpha3"
	"github.com/sudoswedenab/dockyards-flux2/controllers"
	"github.com/sudoswedenab/dockyards-flux2/test/mockcrds"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestDockyardsWorkloadController_Create(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("no kubebuilder assets configured")
	}

	env := envtest.Environment{
		CRDs: mockcrds.CRDs,
	}

	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError})
	slogr := logr.FromSlogHandler(handler)

	ctrl.SetLogger(slogr)

	ctx, cancel := context.WithCancel(context.Background())

	cfg, err := env.Start()
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		cancel()
		env.Stop()
	})

	scheme := runtime.NewScheme()

	_ = corev1.AddToScheme(scheme)
	_ = dockyardsv1.AddToScheme(scheme)
	_ = helmv2.AddToScheme(scheme)
	_ = kustomizev1.AddToScheme(scheme)
	_ = sourcev1.AddToScheme(scheme)

	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}

	namespace := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test-",
		},
	}

	err = c.Create(ctx, &namespace)
	if err != nil {
		t.Fatal(err)
	}

	cluster := dockyardsv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test-",
			Namespace:    namespace.Name,
		},
	}

	err = c.Create(ctx, &cluster)
	if err != nil {
		t.Fatal(err)
	}

	mgr, err := manager.New(cfg, manager.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		err := mgr.Start(ctx)
		if err != nil {
			panic(err)
		}
	}()

	controller := controllers.DockyardsWorkloadReconciler{
		Client: mgr.GetClient(),
	}

	t.Run("test kustomization template", func(t *testing.T) {
		file, err := os.Open("testdata/kustomization.cue")
		if err != nil {
			t.Fatal(err)
		}

		source, err := io.ReadAll(file)
		if err != nil {
			t.Fatal(err)
		}

		workloadTemplate := dockyardsv1.WorkloadTemplate{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-kustomization-",
				Namespace:    namespace.Name,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: dockyardsv1.GroupVersion.String(),
						Kind:       dockyardsv1.ClusterKind,
						Name:       cluster.Name,
						UID:        cluster.UID,
					},
				},
			},
			Spec: dockyardsv1.WorkloadTemplateSpec{
				Source: string(source),
				Type:   dockyardsv1.WorkloadTemplateTypeCue,
			},
		}

		err = c.Create(ctx, &workloadTemplate)
		if err != nil {
			t.Fatal(err)
		}

		workload := dockyardsv1.Workload{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-kustomization-",
				Namespace:    namespace.Name,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: dockyardsv1.GroupVersion.String(),
						Kind:       dockyardsv1.ClusterKind,
						Name:       cluster.Name,
						UID:        cluster.UID,
					},
				},
			},
			Spec: dockyardsv1.WorkloadSpec{
				WorkloadTemplateRef: &corev1.TypedObjectReference{
					Kind:      dockyardsv1.WorkloadTemplateKind,
					Name:      workloadTemplate.Name,
					Namespace: &workloadTemplate.Namespace,
				},
				Provenience: dockyardsv1.ProvenienceDockyards,
			},
		}

		err = c.Create(ctx, &workload)
		if err != nil {
			t.Fatal(err)
		}

		time.Sleep(time.Second)

		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      workload.Name,
				Namespace: workload.Namespace,
			},
		}

		_, err = controller.Reconcile(ctx, req)
		if err != nil {
			t.Fatal(err)
		}

		objectKey := client.ObjectKey{
			Name:      workload.Name,
			Namespace: workload.Namespace,
		}

		var actual kustomizev1.Kustomization
		err = c.Get(ctx, objectKey, &actual)
		if err != nil {
			t.Fatal(err)
		}

		expected := kustomizev1.Kustomization{
			ObjectMeta: actual.ObjectMeta,
			Spec: kustomizev1.KustomizationSpec{
				Interval: metav1.Duration{Duration: time.Minute * 5},
				KubeConfig: &meta.KubeConfigReference{
					SecretRef: meta.SecretKeyReference{
						Name: cluster.Name + "-kubeconfig",
					},
				},
				Path:  "testing",
				Prune: true,
				SourceRef: kustomizev1.CrossNamespaceSourceReference{
					Kind: sourcev1.GitRepositoryKind,
					Name: workload.Name,
				},
			},
			Status: actual.Status,
		}

		if !cmp.Equal(actual, expected) {
			t.Errorf("diff: %s", cmp.Diff(expected, actual))
		}
	})

	t.Run("test helm release template", func(t *testing.T) {
		file, err := os.Open("testdata/helmrelease.cue")
		if err != nil {
			t.Fatal(err)
		}

		source, err := io.ReadAll(file)
		if err != nil {
			t.Fatal(err)
		}

		workloadTemplate := dockyardsv1.WorkloadTemplate{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-helm-release-",
				Namespace:    namespace.Name,
			},
			Spec: dockyardsv1.WorkloadTemplateSpec{
				Source: string(source),
				Type:   dockyardsv1.WorkloadTemplateTypeCue,
			},
		}

		err = c.Create(ctx, &workloadTemplate)
		if err != nil {
			t.Fatal(err)
		}

		workload := dockyardsv1.Workload{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-helm-release-",
				Namespace:    namespace.Name,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: dockyardsv1.GroupVersion.String(),
						Kind:       dockyardsv1.ClusterKind,
						Name:       cluster.Name,
						UID:        cluster.UID,
					},
				},
			},
			Spec: dockyardsv1.WorkloadSpec{
				WorkloadTemplateRef: &corev1.TypedObjectReference{
					Kind:      dockyardsv1.WorkloadTemplateKind,
					Name:      workloadTemplate.Name,
					Namespace: &workloadTemplate.Namespace,
				},
				Provenience:     dockyardsv1.ProvenienceDockyards,
				TargetNamespace: "testing",
			},
		}

		err = c.Create(ctx, &workload)
		if err != nil {
			t.Fatal(err)
		}

		time.Sleep(time.Second)

		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      workload.Name,
				Namespace: workload.Namespace,
			},
		}

		_, err = controller.Reconcile(ctx, req)
		if err != nil {
			t.Fatal(err)
		}

		objectKey := client.ObjectKey{
			Name:      workload.Name,
			Namespace: workload.Namespace,
		}

		var actual helmv2.HelmRelease
		err = c.Get(ctx, objectKey, &actual)
		if err != nil {
			t.Fatal(err)
		}

		raw, err := json.Marshal(map[string]any{
			"replicas": 2,
			"test":     true,
			"nested": map[string]any{
				"name": "testing",
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		expected := helmv2.HelmRelease{
			ObjectMeta: actual.ObjectMeta,
			Spec: helmv2.HelmReleaseSpec{
				Chart: &helmv2.HelmChartTemplate{
					Spec: helmv2.HelmChartTemplateSpec{
						Chart: "testing",
						SourceRef: helmv2.CrossNamespaceObjectReference{
							Kind: sourcev1.HelmRepositoryKind,
							Name: workload.Name,
						},
						Version: "1.2.3",
					},
				},
				Install: &helmv2.Install{
					CreateNamespace: true,
					Remediation: &helmv2.InstallRemediation{
						Retries: -1,
					},
				},
				Interval: metav1.Duration{Duration: time.Minute * 5},
				KubeConfig: &meta.KubeConfigReference{
					SecretRef: meta.SecretKeyReference{
						Name: cluster.Name + "-kubeconfig",
					},
				},
				StorageNamespace: workload.Spec.TargetNamespace,
				TargetNamespace:  workload.Spec.TargetNamespace,
				Values: &apiextensionsv1.JSON{
					Raw: raw,
				},
			},
			Status: actual.Status,
		}

		if !cmp.Equal(actual, expected) {
			t.Errorf("diff: %s", cmp.Diff(expected, actual))
		}
	})

	t.Run("test git repository", func(t *testing.T) {
		file, err := os.Open("testdata/gitrepository.cue")
		if err != nil {
			t.Fatal(err)
		}

		source, err := io.ReadAll(file)
		if err != nil {
			t.Fatal(err)
		}

		workloadTemplate := dockyardsv1.WorkloadTemplate{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-git-repository-",
				Namespace:    namespace.Name,
			},
			Spec: dockyardsv1.WorkloadTemplateSpec{
				Source: string(source),
				Type:   dockyardsv1.WorkloadTemplateTypeCue,
			},
		}

		err = c.Create(ctx, &workloadTemplate)
		if err != nil {
			t.Fatal(err)
		}

		input, err := json.Marshal(map[string]any{
			"url": "https://github.com/stefanprodan/podinfo",
			"ref": map[string]any{
				"branch": "master",
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		workload := dockyardsv1.Workload{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-git-repository-",
				Namespace:    namespace.Name,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: dockyardsv1.GroupVersion.String(),
						Kind:       dockyardsv1.ClusterKind,
						Name:       cluster.Name,
						UID:        cluster.UID,
					},
				},
			},
			Spec: dockyardsv1.WorkloadSpec{
				WorkloadTemplateRef: &corev1.TypedObjectReference{
					Kind:      dockyardsv1.WorkloadTemplateKind,
					Name:      workloadTemplate.Name,
					Namespace: &workloadTemplate.Namespace,
				},
				Provenience: dockyardsv1.ProvenienceDockyards,
				Input: &apiextensionsv1.JSON{
					Raw: input,
				},
			},
		}

		err = c.Create(ctx, &workload)
		if err != nil {
			t.Fatal(err)
		}

		time.Sleep(time.Second)

		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      workload.Name,
				Namespace: workload.Namespace,
			},
		}

		_, err = controller.Reconcile(ctx, req)
		if err != nil {
			t.Fatal(err)
		}

		objectKey := client.ObjectKey{
			Name:      workload.Name,
			Namespace: workload.Namespace,
		}

		var actual sourcev1.GitRepository
		err = c.Get(ctx, objectKey, &actual)
		if err != nil {
			t.Fatal(err)
		}

		expected := sourcev1.GitRepository{
			Spec: sourcev1.GitRepositorySpec{
				Reference: &sourcev1.GitRepositoryRef{
					Branch: "main",
				},
			},
		}

		if !cmp.Equal(actual, expected) {
			cmp.Diff("diff: %s", cmp.Diff(expected, actual))
		}
	})

	t.Run("test dockyards worktree", func(t *testing.T) {
		file, err := os.Open("testdata/dockyardsworktree.cue")
		if err != nil {
			t.Fatal(err)
		}

		source, err := io.ReadAll(file)
		if err != nil {
			t.Fatal(err)
		}

		workloadTemplate := dockyardsv1.WorkloadTemplate{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-worktree-",
				Namespace:    namespace.Name,
			},
			Spec: dockyardsv1.WorkloadTemplateSpec{
				Source: string(source),
				Type:   dockyardsv1.WorkloadTemplateTypeCue,
			},
		}

		err = c.Create(ctx, &workloadTemplate)
		if err != nil {
			t.Fatal(err)
		}

		workload := dockyardsv1.Workload{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-worktree-",
				Namespace:    namespace.Name,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: dockyardsv1.GroupVersion.String(),
						Kind:       dockyardsv1.ClusterKind,
						Name:       cluster.Name,
						UID:        cluster.UID,
					},
				},
			},
			Spec: dockyardsv1.WorkloadSpec{
				Provenience: dockyardsv1.ProvenienceUser,
				WorkloadTemplateRef: &corev1.TypedObjectReference{
					Kind: dockyardsv1.WorkloadTemplateKind,
					Name: workloadTemplate.Name,
				},
			},
		}

		err = c.Create(ctx, &workload)
		if err != nil {
			t.Fatal(err)
		}

		time.Sleep(time.Second)

		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      workload.Name,
				Namespace: workload.Namespace,
			},
		}

		_, err = controller.Reconcile(ctx, req)
		if err != nil {
			t.Fatal(err)
		}

		var actual dockyardsv1.Worktree
		err = c.Get(ctx, client.ObjectKeyFromObject(&workload), &actual)
		if err != nil {
			t.Fatal(err)
		}

		expected := dockyardsv1.Worktree{
			ObjectMeta: metav1.ObjectMeta{
				Name:      workload.Name,
				Namespace: workload.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion:         dockyardsv1.GroupVersion.String(),
						Kind:               dockyardsv1.WorkloadKind,
						Name:               workload.Name,
						UID:                workload.UID,
						Controller:         ptr.To(true),
						BlockOwnerDeletion: ptr.To(true),
					},
				},
				Labels: map[string]string{
					dockyardsv1.LabelWorkloadName: workload.Name,
					dockyardsv1.LabelClusterName:  cluster.Name,
				},
				CreationTimestamp: actual.CreationTimestamp,
				Generation:        actual.Generation,
				ManagedFields:     actual.ManagedFields,
				ResourceVersion:   actual.ResourceVersion,
				UID:               actual.UID,
			},
			Spec: dockyardsv1.WorktreeSpec{
				Files: map[string][]byte{
					"test": []byte("hello"),
				},
			},
		}

		if !cmp.Equal(actual, expected) {
			t.Errorf("diff: %s", cmp.Diff(expected, actual))
		}
	})

	t.Run("test secret", func(t *testing.T) {
		file, err := os.Open("testdata/secret.cue")
		if err != nil {
			t.Fatal(err)
		}

		source, err := io.ReadAll(file)
		if err != nil {
			t.Fatal(err)
		}

		workloadTemplate := dockyardsv1.WorkloadTemplate{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-worktree-",
				Namespace:    namespace.Name,
			},
			Spec: dockyardsv1.WorkloadTemplateSpec{
				Source: string(source),
				Type:   dockyardsv1.WorkloadTemplateTypeCue,
			},
		}

		err = c.Create(ctx, &workloadTemplate)
		if err != nil {
			t.Fatal(err)
		}

		workload := dockyardsv1.Workload{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-worktree-",
				Namespace:    namespace.Name,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: dockyardsv1.GroupVersion.String(),
						Kind:       dockyardsv1.ClusterKind,
						Name:       cluster.Name,
						UID:        cluster.UID,
					},
				},
			},
			Spec: dockyardsv1.WorkloadSpec{
				Provenience: dockyardsv1.ProvenienceUser,
				WorkloadTemplateRef: &corev1.TypedObjectReference{
					Kind: dockyardsv1.WorkloadTemplateKind,
					Name: workloadTemplate.Name,
				},
			},
		}

		err = c.Create(ctx, &workload)
		if err != nil {
			t.Fatal(err)
		}

		time.Sleep(time.Second)

		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      workload.Name,
				Namespace: workload.Namespace,
			},
		}

		_, err = controller.Reconcile(ctx, req)
		if err != nil {
			t.Fatal(err)
		}

		var actual corev1.Secret
		err = c.Get(ctx, client.ObjectKeyFromObject(&workload), &actual)
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestDockyardsWorkloadController_Update(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("no kubebuilder assets configured")
	}

	env := envtest.Environment{
		CRDs: mockcrds.CRDs,
	}

	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	slogr := logr.FromSlogHandler(handler)

	ctrl.SetLogger(slogr)

	ctx, cancel := context.WithCancel(context.Background())

	cfg, err := env.Start()
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		cancel()
		env.Stop()
	})

	scheme := runtime.NewScheme()

	_ = corev1.AddToScheme(scheme)
	_ = dockyardsv1.AddToScheme(scheme)
	_ = helmv2.AddToScheme(scheme)
	_ = kustomizev1.AddToScheme(scheme)
	_ = sourcev1.AddToScheme(scheme)

	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}

	namespace := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test-",
		},
	}

	err = c.Create(ctx, &namespace)
	if err != nil {
		t.Fatal(err)
	}

	cluster := dockyardsv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test-",
			Namespace:    namespace.Name,
		},
	}

	err = c.Create(ctx, &cluster)
	if err != nil {
		t.Fatal(err)
	}

	mgr, err := manager.New(cfg, manager.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		err := mgr.Start(ctx)
		if err != nil {
			panic(err)
		}
	}()

	controller := controllers.DockyardsWorkloadReconciler{
		Client: mgr.GetClient(),
	}

	t.Run("test template create", func(t *testing.T) {
		file, err := os.Open("testdata/template_create.cue")
		if err != nil {
			t.Fatal(err)
		}

		source, err := io.ReadAll(file)
		if err != nil {
			t.Fatal(err)
		}

		workloadTemplate := dockyardsv1.WorkloadTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-update",
				Namespace: namespace.Name,
			},
			Spec: dockyardsv1.WorkloadTemplateSpec{
				Source: string(source),
				Type:   dockyardsv1.WorkloadTemplateTypeCue,
			},
		}

		err = c.Create(ctx, &workloadTemplate)
		if err != nil {
			t.Fatal(err)
		}

		workload := dockyardsv1.Workload{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-update",
				Namespace: namespace.Name,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: dockyardsv1.GroupVersion.String(),
						Kind:       dockyardsv1.ClusterKind,
						Name:       cluster.Name,
						UID:        cluster.UID,
					},
				},
			},
			Spec: dockyardsv1.WorkloadSpec{
				WorkloadTemplateRef: &corev1.TypedObjectReference{
					Kind:      dockyardsv1.WorkloadKind,
					Name:      workloadTemplate.Name,
					Namespace: &workloadTemplate.Namespace,
				},
				Provenience: dockyardsv1.ProvenienceUser,
			},
		}

		err = c.Create(ctx, &workload)
		if err != nil {
			t.Fatal(err)
		}

		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      workload.Name,
				Namespace: workload.Namespace,
			},
		}

		_, err = controller.Reconcile(ctx, req)
		if err != nil {
			t.Fatal(err)
		}

		objectKey := client.ObjectKey{
			Name:      cluster.Name + "-" + workload.Name,
			Namespace: workload.Namespace,
		}

		var actual sourcev1.GitRepository
		err = c.Get(ctx, objectKey, &actual)
		if err != nil {
			t.Fatal(err)
		}

		expected := sourcev1.GitRepository{
			ObjectMeta: actual.ObjectMeta,
			Spec: sourcev1.GitRepositorySpec{
				Interval: metav1.Duration{Duration: time.Minute * 5},
				URL:      "https://github.com/stefanprodan/podinfo",
				Reference: &sourcev1.GitRepositoryRef{
					Branch: "master",
				},
			},
			Status: actual.Status,
		}

		if !cmp.Equal(actual, expected) {
			t.Errorf("diff: %s", cmp.Diff(expected, actual))
		}
	})

	t.Run("test template update", func(t *testing.T) {
		file, err := os.Open("testdata/template_update.cue")
		if err != nil {
			t.Fatal(err)
		}

		source, err := io.ReadAll(file)
		if err != nil {
			t.Fatal(err)
		}

		objectKey := client.ObjectKey{
			Name:      "test-update",
			Namespace: namespace.Name,
		}

		var workloadTemplate dockyardsv1.WorkloadTemplate
		err = c.Get(ctx, objectKey, &workloadTemplate)
		if err != nil {
			t.Fatal(err)
		}

		patch := client.MergeFrom(workloadTemplate.DeepCopy())

		workloadTemplate.Spec.Source = string(source)

		err = c.Patch(ctx, &workloadTemplate, patch)
		if err != nil {
			t.Fatal(err)
		}

		err = wait.ExponentialBackoff(retry.DefaultRetry, func() (bool, error) {
			err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(&workloadTemplate), &workloadTemplate)
			if err != nil {
				return true, err
			}

			if workloadTemplate.Spec.Source == string(source) {
				return true, nil
			}

			return false, nil
		})
		if err != nil {
			t.Fatal(err)
		}

		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "test-update",
				Namespace: namespace.Name,
			},
		}

		_, err = controller.Reconcile(ctx, req)
		if err != nil {
			t.Fatal(err)
		}

		objectKey = client.ObjectKey{
			Name:      cluster.Name + "-test-update",
			Namespace: namespace.Name,
		}

		var actual sourcev1.GitRepository
		err = c.Get(ctx, objectKey, &actual)
		if err != nil {
			t.Fatal(err)
		}

		expected := sourcev1.GitRepository{
			ObjectMeta: actual.ObjectMeta,
			Spec: sourcev1.GitRepositorySpec{
				Interval: metav1.Duration{Duration: time.Minute * 5},
				URL:      "https://github.com/stefanprodan/podinfo",
				Reference: &sourcev1.GitRepositoryRef{
					Branch: "main",
				},
			},
			Status: actual.Status,
		}

		if !cmp.Equal(actual, expected) {
			t.Errorf("diff: %s", cmp.Diff(expected, actual))
		}
	})
}
