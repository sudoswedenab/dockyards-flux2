package controller

import (
	"context"
	"log/slog"
	"time"

	"bitbucket.org/sudosweden/dockyards-backend/pkg/api/apiutil"
	dockyardsv1alpha1 "bitbucket.org/sudosweden/dockyards-backend/pkg/api/v1alpha1"
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	fluxcdmeta "github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/source-controller/api/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// +kubebuilder:rbac:groups=dockyards.io,resources=clusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=dockyards.io,resources=deployments,verbs=get;list;patch;watch
// +kubebuilder:rbac:groups=dockyards.io,resources=deployments/status,verbs=patch
// +kubebuilder:rbac:groups=dockyards.io,resources=kustomizedeployments,verbs=get;list;watch
// +kubebuilder:rbac:groups=kustomize.toolkit.fluxcd.io,resources=kustomizations,verbs=create;get;list;watch
// +kubebuilder:rbac:groups=source.toolkit.fluxcd.io,resources=gitrepositories,verbs=create;get;list;watch

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

	controller := true

	gitRepository := v1.GitRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kustomizeDeployment.Name,
			Namespace: kustomizeDeployment.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: dockyardsv1alpha1.GroupVersion.String(),
					Kind:       dockyardsv1alpha1.KustomizeDeploymentKind,
					Name:       kustomizeDeployment.Name,
					UID:        kustomizeDeployment.UID,
					Controller: &controller,
				},
			},
		},
	}

	operationResult, err := controllerutil.CreateOrPatch(ctx, r.Client, &gitRepository, func() error {
		gitRepository.Spec.Interval = metav1.Duration{Duration: time.Minute * 5}
		gitRepository.Spec.URL = kustomizeDeployment.Status.RepositoryURL
		gitRepository.Spec.Reference = &v1.GitRepositoryRef{
			Branch: "main",
		}

		return nil
	})
	if err != nil {
		logger.Error("error reconciling git repository", "err", err)

		return ctrl.Result{}, err
	}

	logger.Debug("reconciled git repository", "result", operationResult)

	kustomization := kustomizev1.Kustomization{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kustomizeDeployment.Name,
			Namespace: kustomizeDeployment.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: dockyardsv1alpha1.GroupVersion.String(),
					Kind:       dockyardsv1alpha1.KustomizeDeploymentKind,
					Name:       kustomizeDeployment.Name,
					UID:        kustomizeDeployment.UID,
					Controller: &controller,
				},
			},
		},
	}

	operationResult, err = controllerutil.CreateOrPatch(ctx, r.Client, &kustomization, func() error {
		kustomization.Spec.Interval = metav1.Duration{Duration: time.Minute * 10}
		kustomization.Spec.TargetNamespace = ownerDeployment.Spec.TargetNamespace
		kustomization.Spec.SourceRef = kustomizev1.CrossNamespaceSourceReference{
			APIVersion: v1.GroupVersion.String(),
			Kind:       v1.GitRepositoryKind,
			Name:       gitRepository.Name,
		}
		kustomization.Spec.Prune = true
		kustomization.Spec.Timeout = &metav1.Duration{Duration: time.Minute}
		kustomization.Spec.KubeConfig = &fluxcdmeta.KubeConfigReference{
			SecretRef: fluxcdmeta.SecretKeyReference{
				Name: ownerCluster.Name + "-kubeconfig",
			},
		}

		return nil
	})
	if err != nil {
		logger.Error("error reconciling kustomization", "err", err)

		return ctrl.Result{}, err
	}

	logger.Debug("reconciled kustomization", "result", operationResult)

	if ownerDeployment.Spec.DeploymentRef.Name == "" {
		logger.Debug("owner deployment reference empty")

		patch := client.MergeFrom(ownerDeployment.DeepCopy())

		ownerDeployment.Spec.DeploymentRef = dockyardsv1alpha1.DeploymentReference{
			APIVersion: dockyardsv1alpha1.GroupVersion.String(),
			Kind:       dockyardsv1alpha1.KustomizeDeploymentKind,
			Name:       kustomizeDeployment.Name,
			UID:        kustomizeDeployment.UID,
		}

		err := r.Patch(ctx, ownerDeployment, patch)
		if err != nil {
			logger.Error("error patching owner deployment")

			return ctrl.Result{}, err
		}
	}

	kustomizationReadyCondition := meta.FindStatusCondition(kustomization.Status.Conditions, fluxcdmeta.ReadyCondition)
	if kustomizationReadyCondition == nil {
		logger.Debug("kustomization has no ready condition")

		return ctrl.Result{}, nil
	}

	if !meta.IsStatusConditionPresentAndEqual(ownerDeployment.Status.Conditions, dockyardsv1alpha1.ReadyCondition, kustomizationReadyCondition.Status) {
		logger.Debug("owner deployment needs status condition update")

		readyCondition := metav1.Condition{
			Type:    dockyardsv1alpha1.ReadyCondition,
			Status:  kustomizationReadyCondition.Status,
			Message: kustomizationReadyCondition.Message,
			Reason:  kustomizationReadyCondition.Reason,
		}

		patch := client.MergeFrom(ownerDeployment.DeepCopy())

		meta.SetStatusCondition(&ownerDeployment.Status.Conditions, readyCondition)

		err := r.Status().Patch(ctx, ownerDeployment, patch)
		if err != nil {
			logger.Error("error patching owner deployment", "err", err)

			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *KustomizeDeploymentReconciler) SetupWithManager(manager ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(manager).
		For(&dockyardsv1alpha1.KustomizeDeployment{}).
		Owns(&v1.GitRepository{}).
		Owns(&kustomizev1.Kustomization{}).
		Complete(r)
}
