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

	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	dockyardsv1 "github.com/sudoswedenab/dockyards-backend/api/v1alpha3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// +kubebuilder:rbac:groups=dockyards.io,resources=workloadinventories,verbs=create;get;list;patch;watch
// +kubebuilder:rbac:groups=helm.toolkit.fluxcd.io,resources=helmreleases,verbs=get;list;watch

const (
	HelmReleaseSuffix = "-lpfhh"
)

type HelmReleaseReconciler struct {
	client.Client
}

func (r *HelmReleaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var helmRelease helmv2.HelmRelease
	err := r.Get(ctx, req.NamespacedName, &helmRelease)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !helmRelease.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	result, err := r.reconcileWorkloadInventory(ctx, &helmRelease)
	if err != nil {
		return result, err
	}

	return ctrl.Result{}, nil
}

func (r *HelmReleaseReconciler) reconcileWorkloadInventory(ctx context.Context, helmRelease *helmv2.HelmRelease) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	clusterName, hasLabel := helmRelease.Labels[dockyardsv1.LabelClusterName]
	if !hasLabel {
		return ctrl.Result{}, nil
	}

	workloadName, hasLabel := helmRelease.Labels[dockyardsv1.LabelWorkloadName]
	if !hasLabel {
		return ctrl.Result{}, nil
	}

	workloadInventory := dockyardsv1.WorkloadInventory{
		ObjectMeta: metav1.ObjectMeta{
			Name:      helmRelease.Name + HelmReleaseSuffix,
			Namespace: helmRelease.Namespace,
		},
	}

	operationResult, err := controllerutil.CreateOrPatch(ctx, r, &workloadInventory, func() error {
		workloadInventory.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: helmv2.GroupVersion.String(),
				Kind:       helmv2.HelmReleaseKind,
				Name:       helmRelease.Name,
				UID:        helmRelease.UID,
			},
		}

		if workloadInventory.Labels == nil {
			workloadInventory.Labels = make(map[string]string)
		}

		workloadInventory.Labels[dockyardsv1.LabelClusterName] = clusterName
		workloadInventory.Labels[dockyardsv1.LabelWorkloadName] = workloadName

		workloadInventory.Spec.Selector = metav1.LabelSelector{
			MatchLabels: map[string]string{
				"app.kubernetes.io/managed-by":     "Helm",
				"app.kubernetes.io/name":           helmRelease.Spec.Chart.Spec.Chart,
				"app.kubernetes.io/version":        helmRelease.Spec.Chart.Spec.Version,
				"helm.sh/chart":                    helmRelease.Spec.Chart.Spec.Chart + "-" + helmRelease.Spec.Chart.Spec.Version,
				"helm.toolkit.fluxcd.io/name":      helmRelease.Name,
				"helm.toolkit.fluxcd.io/namespace": helmRelease.Namespace,
			},
		}

		return nil
	})
	if err != nil {
		return ctrl.Result{}, nil
	}

	if operationResult != controllerutil.OperationResultNone {
		logger.Info("reconciled workload inventory", "inventoryName", workloadInventory.Name)
	}

	return ctrl.Result{}, nil
}

func (r *HelmReleaseReconciler) SetupWithManager(m ctrl.Manager) error {
	scheme := m.GetScheme()

	_ = dockyardsv1.AddToScheme(scheme)
	_ = helmv2.AddToScheme(scheme)

	err := ctrl.NewControllerManagedBy(m).For(&helmv2.HelmRelease{}).Complete(r)
	if err != nil {
		return err
	}

	return nil
}
