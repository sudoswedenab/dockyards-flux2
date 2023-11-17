package controller

import (
	"context"
	"log/slog"
	"time"

	"bitbucket.org/sudosweden/dockyards-backend/pkg/api/apiutil"
	dockyardsv1alpha1 "bitbucket.org/sudosweden/dockyards-backend/pkg/api/v1alpha1"
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/source-controller/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type KustomizeDeploymentReconciler struct {
	client.Client
	Logger *slog.Logger
}

func (r *KustomizeDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var kustomizeDeployment dockyardsv1alpha1.KustomizeDeployment
	err := r.Get(ctx, req.NamespacedName, &kustomizeDeployment)
	if client.IgnoreNotFound(err) != nil {
		r.Logger.Error("error getting kustomize deployment", "err", err)
	}

	logger := r.Logger.With("name", kustomizeDeployment.Name, "namespace", kustomizeDeployment.Namespace)
	logger.Debug("reconcile kustomize deployment")

	if kustomizeDeployment.Status.RepositoryURL == "" {
		logger.Info("ignoring kustomize deployment without repository url")

		return ctrl.Result{}, nil
	}

	ownerDeployment, err := GetOwnerDeployment(ctx, r.Client, &kustomizeDeployment)
	if err != nil {
		logger.Error("error getting owner deployment", "err", err)

		return ctrl.Result{}, err
	}

	if ownerDeployment == nil {
		logger.Info("ignoring kustomize deployment without owner deployment")

		return ctrl.Result{}, nil
	}

	ownerCluster, err := apiutil.GetOwnerCluster(ctx, r.Client, ownerDeployment)
	if err != nil {
		logger.Error("error getting owner cluster")

		return ctrl.Result{}, err
	}

	if ownerCluster == nil {
		logger.Info("ignoring kustomize deployment without owner cluster")

		return ctrl.Result{}, nil
	}

	var gitRepository v1.GitRepository
	err = r.Get(ctx, req.NamespacedName, &gitRepository)
	if client.IgnoreNotFound(err) != nil {
		logger.Error("error getting git repository", "err", err)

		return ctrl.Result{}, err
	}

	if apierrors.IsNotFound(err) {
		logger.Debug("git repository not found")

		gitRepository = v1.GitRepository{
			ObjectMeta: metav1.ObjectMeta{
				Name:      kustomizeDeployment.Name,
				Namespace: kustomizeDeployment.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: dockyardsv1alpha1.GroupVersion.String(),
						Kind:       dockyardsv1alpha1.KustomizeDeploymentKind,
						Name:       kustomizeDeployment.Name,
						UID:        kustomizeDeployment.UID,
					},
				},
			},
			Spec: v1.GitRepositorySpec{
				Interval: metav1.Duration{Duration: time.Minute * 5},
				URL:      kustomizeDeployment.Status.RepositoryURL,
				Reference: &v1.GitRepositoryRef{
					Branch: "main",
				},
			},
		}

		err := r.Create(ctx, &gitRepository)
		if err != nil {
			logger.Error("error creating git repository", "err", err)

			return ctrl.Result{}, err
		}
	}

	var kustomization kustomizev1.Kustomization
	err = r.Get(ctx, req.NamespacedName, &kustomization)
	if client.IgnoreNotFound(err) != nil {
		logger.Error("error getting kustomization", "err", err)

		return ctrl.Result{}, err
	}

	if apierrors.IsNotFound(err) {
		logger.Debug("kustomization not found")

		kustomization = kustomizev1.Kustomization{
			ObjectMeta: metav1.ObjectMeta{
				Name:      kustomizeDeployment.Name,
				Namespace: kustomizeDeployment.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: dockyardsv1alpha1.GroupVersion.String(),
						Kind:       dockyardsv1alpha1.KustomizeDeploymentKind,
						Name:       kustomizeDeployment.Name,
						UID:        kustomizeDeployment.UID,
					},
				},
			},
			Spec: kustomizev1.KustomizationSpec{
				Interval:        metav1.Duration{Duration: time.Minute * 10},
				TargetNamespace: ownerDeployment.Spec.TargetNamespace,
				SourceRef: kustomizev1.CrossNamespaceSourceReference{
					APIVersion: v1.GroupVersion.String(),
					Kind:       v1.GitRepositoryKind,
					Name:       gitRepository.Name,
				},
				Prune:   true,
				Timeout: &metav1.Duration{Duration: time.Minute},
				KubeConfig: &meta.KubeConfigReference{
					SecretRef: meta.SecretKeyReference{
						Name: ownerCluster.Name + "-kubeconfig",
					},
				},
			},
		}

		err := r.Create(ctx, &kustomization)
		if err != nil {
			logger.Error("error creating kustomization", "err", err)

			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *KustomizeDeploymentReconciler) SetupWithManager(manager ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(manager).For(&dockyardsv1alpha1.KustomizeDeployment{}).Complete(r)
}
