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

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	dockyardsv1 "github.com/sudoswedenab/dockyards-backend/api/v1alpha3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

// +kubebuilder:rbac:groups=dockyards.io,resources=workloadinventories,verbs=create;get;list;patch;watch
// +kubebuilder:rbac:groups=source.toolkit.fluxcd.io,resources=helmrepositories;ocirepositories,verbs=get;list;watch

const (
	HelmRepositorySuffix = "-j3w8d"
	OCIRepositorySuffix  = "-v5f7k"
)

type SourceRepositoryReconciler struct {
	client.Client
}

func (r *SourceRepositoryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var helmRepository sourcev1.HelmRepository
	helmRepositoryErr := r.Get(ctx, req.NamespacedName, &helmRepository)
	if client.IgnoreNotFound(helmRepositoryErr) != nil {
		return ctrl.Result{}, helmRepositoryErr
	}

	if helmRepositoryErr == nil && helmRepository.DeletionTimestamp.IsZero() {
		result, err := r.reconcileHelmRepositoryWorkloadInventory(ctx, &helmRepository)
		if err != nil {
			return result, err
		}
	}

	var ociRepository sourcev1.OCIRepository
	ociRepositoryErr := r.Get(ctx, req.NamespacedName, &ociRepository)
	if client.IgnoreNotFound(ociRepositoryErr) != nil {
		return ctrl.Result{}, ociRepositoryErr
	}

	if ociRepositoryErr == nil && ociRepository.DeletionTimestamp.IsZero() {
		result, err := r.reconcileOCIRepositoryWorkloadInventory(ctx, &ociRepository)
		if err != nil {
			return result, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *SourceRepositoryReconciler) reconcileHelmRepositoryWorkloadInventory(ctx context.Context, helmRepository *sourcev1.HelmRepository) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	clusterName, hasLabel := helmRepository.Labels[dockyardsv1.LabelClusterName]
	if !hasLabel {
		return ctrl.Result{}, nil
	}

	workloadName, hasLabel := helmRepository.Labels[dockyardsv1.LabelWorkloadName]
	if !hasLabel {
		return ctrl.Result{}, nil
	}

	workloadInventory := dockyardsv1.WorkloadInventory{
		ObjectMeta: metav1.ObjectMeta{
			Name:      helmRepository.Name + HelmRepositorySuffix,
			Namespace: helmRepository.Namespace,
		},
	}

	operationResult, err := controllerutil.CreateOrPatch(ctx, r, &workloadInventory, func() error {
		workloadInventory.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: sourcev1.GroupVersion.String(),
				Kind:       sourcev1.HelmRepositoryKind,
				Name:       helmRepository.Name,
				UID:        helmRepository.UID,
			},
		}

		if workloadInventory.Labels == nil {
			workloadInventory.Labels = make(map[string]string)
		}

		workloadInventory.Labels[dockyardsv1.LabelClusterName] = clusterName
		workloadInventory.Labels[dockyardsv1.LabelWorkloadName] = workloadName

		workloadInventory.Spec.Selector = metav1.LabelSelector{
			MatchLabels: map[string]string{
				"source.toolkit.fluxcd.io/name":      helmRepository.Name,
				"source.toolkit.fluxcd.io/namespace": helmRepository.Namespace,
			},
		}

		return nil
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	if operationResult != controllerutil.OperationResultNone {
		logger.Info("reconciled workload inventory", "inventoryName", workloadInventory.Name)
	}

	return ctrl.Result{}, nil
}

func (r *SourceRepositoryReconciler) reconcileOCIRepositoryWorkloadInventory(ctx context.Context, ociRepository *sourcev1.OCIRepository) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	clusterName, hasLabel := ociRepository.Labels[dockyardsv1.LabelClusterName]
	if !hasLabel {
		return ctrl.Result{}, nil
	}

	workloadName, hasLabel := ociRepository.Labels[dockyardsv1.LabelWorkloadName]
	if !hasLabel {
		return ctrl.Result{}, nil
	}

	workloadInventory := dockyardsv1.WorkloadInventory{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ociRepository.Name + OCIRepositorySuffix,
			Namespace: ociRepository.Namespace,
		},
	}

	operationResult, err := controllerutil.CreateOrPatch(ctx, r, &workloadInventory, func() error {
		workloadInventory.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: sourcev1.GroupVersion.String(),
				Kind:       sourcev1.OCIRepositoryKind,
				Name:       ociRepository.Name,
				UID:        ociRepository.UID,
			},
		}

		if workloadInventory.Labels == nil {
			workloadInventory.Labels = make(map[string]string)
		}

		workloadInventory.Labels[dockyardsv1.LabelClusterName] = clusterName
		workloadInventory.Labels[dockyardsv1.LabelWorkloadName] = workloadName

		workloadInventory.Spec.Selector = metav1.LabelSelector{
			MatchLabels: map[string]string{
				"source.toolkit.fluxcd.io/name":      ociRepository.Name,
				"source.toolkit.fluxcd.io/namespace": ociRepository.Namespace,
			},
		}

		return nil
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	if operationResult != controllerutil.OperationResultNone {
		logger.Info("reconciled workload inventory", "inventoryName", workloadInventory.Name)
	}

	return ctrl.Result{}, nil
}

func (r *SourceRepositoryReconciler) objectToRequest(_ context.Context, obj client.Object) []ctrl.Request {
	return []ctrl.Request{
		{
			NamespacedName: types.NamespacedName{
				Name:      obj.GetName(),
				Namespace: obj.GetNamespace(),
			},
		},
	}
}

func (r *SourceRepositoryReconciler) SetupWithManager(m ctrl.Manager) error {
	scheme := m.GetScheme()

	_ = dockyardsv1.AddToScheme(scheme)
	_ = sourcev1.AddToScheme(scheme)

	err := ctrl.NewControllerManagedBy(m).
		For(&sourcev1.HelmRepository{}).
		Watches(
			&sourcev1.OCIRepository{},
			handler.EnqueueRequestsFromMapFunc(r.objectToRequest),
		).
		Complete(r)
	if err != nil {
		return err
	}

	return nil
}
