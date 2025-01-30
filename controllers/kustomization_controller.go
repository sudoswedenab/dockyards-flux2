package controllers

import (
	"context"

	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	"github.com/fluxcd/pkg/runtime/patch"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=kustomize.toolkit.fluxcd.io,resources=kustomizations,verbs=get;list;patch;watch

type KustomizationReconciler struct {
	client.Client
}

func (r *KustomizationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, reterr error) {
	logger := ctrl.LoggerFrom(ctx)

	var kustomization kustomizev1.Kustomization
	err := r.Get(ctx, req.NamespacedName, &kustomization)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if kustomization.DeletionTimestamp.IsZero() || kustomization.Spec.KubeConfig == nil || !kustomization.Spec.Prune {
		return ctrl.Result{}, nil
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

	objectKey := client.ObjectKey{
		Name:      kustomization.Spec.KubeConfig.SecretRef.Name,
		Namespace: kustomization.Namespace,
	}

	var secret corev1.Secret
	err = r.Get(ctx, objectKey, &secret)
	if client.IgnoreNotFound(err) != nil {
		return ctrl.Result{}, err
	}

	if apierrors.IsNotFound(err) {
		kustomization.Spec.Prune = false

		logger.Info("updating prune flag for kustomization with no kubeconfig secret", "prune", kustomization.Spec.Prune)
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
