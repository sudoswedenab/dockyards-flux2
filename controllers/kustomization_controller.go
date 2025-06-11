package controllers

import (
	"context"

	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	"github.com/fluxcd/pkg/runtime/patch"
	dockyardsv1 "github.com/sudoswedenab/dockyards-backend/api/v1alpha3"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=dockyards.io,resources=workloadinventories,verbs=create;get;list;patch;watch
// +kubebuilder:rbac:groups=kustomize.toolkit.fluxcd.io,resources=kustomizations,verbs=get;list;patch;watch

const (
	KustomizationSuffix = "-4l8sj"
)

type KustomizationReconciler struct {
	client.Client
}

func (r *KustomizationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, reterr error) {
	var kustomization kustomizev1.Kustomization
	err := r.Get(ctx, req.NamespacedName, &kustomization)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	patchHelper, err := patch.NewHelper(&kustomization, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	defer func() {
		err := patchHelper.Patch(ctx, &kustomization)
		if err != nil {
			result = ctrl.Result{}
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}
	}()

	if !kustomization.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &kustomization)
	}

	result, err = r.reconcileWorkloadInventory(ctx, &kustomization)
	if err != nil {
		return result, err
	}

	return ctrl.Result{}, nil
}

func (r *KustomizationReconciler) reconcileDelete(ctx context.Context, kustomization *kustomizev1.Kustomization) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	if kustomization.Spec.KubeConfig == nil || !kustomization.Spec.Prune {
		return ctrl.Result{}, nil
	}

	objectKey := client.ObjectKey{
		Name:      kustomization.Spec.KubeConfig.SecretRef.Name,
		Namespace: kustomization.Namespace,
	}

	var secret corev1.Secret
	err := r.Get(ctx, objectKey, &secret)
	if client.IgnoreNotFound(err) != nil {
		return ctrl.Result{}, err
	}

	if apierrors.IsNotFound(err) {
		kustomization.Spec.Prune = false

		logger.Info("updating prune flag for kustomization with no kubeconfig secret", "prune", kustomization.Spec.Prune)
	}

	return ctrl.Result{}, nil
}

func (r *KustomizationReconciler) reconcileWorkloadInventory(ctx context.Context, kustomization *kustomizev1.Kustomization) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	workloadName, hasLabel := kustomization.Labels[dockyardsv1.LabelWorkloadName]
	if !hasLabel {
		return ctrl.Result{}, nil
	}

	clusterName, hasLabel := kustomization.Labels[dockyardsv1.LabelClusterName]
	if !hasLabel {
		return ctrl.Result{}, nil
	}

	workloadInventory := dockyardsv1.WorkloadInventory{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kustomization.Name + KustomizationSuffix,
			Namespace: kustomization.Namespace,
		},
	}

	operationResult, err := controllerutil.CreateOrPatch(ctx, r, &workloadInventory, func() error {
		workloadInventory.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: kustomizev1.GroupVersion.String(),
				Kind:       kustomizev1.KustomizationKind,
				Name:       kustomization.Name,
				UID:        kustomization.UID,
			},
		}

		if workloadInventory.Labels == nil {
			workloadInventory.Labels = make(map[string]string)
		}

		workloadInventory.Labels[dockyardsv1.LabelWorkloadName] = workloadName
		workloadInventory.Labels[dockyardsv1.LabelClusterName] = clusterName

		workloadInventory.Spec.Selector = metav1.LabelSelector{
			MatchLabels: map[string]string{
				"kustomize.toolkit.fluxcd.io/name":      kustomization.Name,
				"kustomize.toolkit.fluxcd.io/namespace": kustomization.Namespace,
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

func (r *KustomizationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	scheme := mgr.GetScheme()

	_ = corev1.AddToScheme(scheme)
	_ = kustomizev1.AddToScheme(scheme)

	err := ctrl.NewControllerManagedBy(mgr).For(&kustomizev1.Kustomization{}).Complete(r)
	if err != nil {
		return err
	}

	return nil
}
