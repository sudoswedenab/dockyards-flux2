package controller

import (
	"context"
	"time"

	"bitbucket.org/sudosweden/dockyards-backend/pkg/api/apiutil"
	dockyardsv1alpha1 "bitbucket.org/sudosweden/dockyards-backend/pkg/api/v1alpha1"
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	fluxcdmeta "github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/source-controller/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// +kubebuilder:rbac:groups=dockyards.io,resources=clusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=dockyards.io,resources=containerimagedeployments,verbs=get;list;watch
// +kubebuilder:rbac:groups=dockyards.io,resources=deployments,verbs=get;list;patch;watch
// +kubebuilder:rbac:groups=dockyards.io,resources=deployments/status,verbs=patch
// +kubebuilder:rbac:groups=kustomize.toolkit.fluxcd.io,resources=kustomizations,verbs=create;get;list;watch
// +kubebuilder:rbac:groups=source.toolkit.fluxcd.io,resources=gitrepositories,verbs=create;get;list;watch

type ContainerImageDeploymentReconciler struct {
	client.Client
}

func (r *ContainerImageDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	var containerImageDeployment dockyardsv1alpha1.ContainerImageDeployment
	err := r.Get(ctx, req.NamespacedName, &containerImageDeployment)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !containerImageDeployment.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	ownerDeployment, err := GetOwnerDeployment(ctx, r.Client, &containerImageDeployment)
	if err != nil {
		logger.Error(err, "error getting owner deployment")

		return ctrl.Result{}, err
	}

	if ownerDeployment == nil {
		logger.Info("ignoring container image deployment without owner")

		return ctrl.Result{}, nil
	}

	ownerCluster, err := apiutil.GetOwnerCluster(ctx, r.Client, ownerDeployment)

	if containerImageDeployment.Status.RepositoryURL == "" {
		logger.Info("ignoring container image deployment without repository url")

		return ctrl.Result{}, nil
	}

	var gitRepository v1.GitRepository
	err = r.Get(ctx, req.NamespacedName, &gitRepository)
	if client.IgnoreNotFound(err) != nil {
		logger.Error(err, "error getting git repository")

		return ctrl.Result{}, err
	}

	if apierrors.IsNotFound(err) {
		logger.Info("git repository not found")

		gitRepository = v1.GitRepository{
			ObjectMeta: metav1.ObjectMeta{
				Name:      containerImageDeployment.Name,
				Namespace: containerImageDeployment.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: dockyardsv1alpha1.GroupVersion.String(),
						Kind:       dockyardsv1alpha1.ContainerImageDeploymentKind,
						Name:       containerImageDeployment.Name,
						UID:        containerImageDeployment.UID,
					},
				},
			},
			Spec: v1.GitRepositorySpec{
				Interval: metav1.Duration{Duration: time.Minute * 5},
				URL:      containerImageDeployment.Status.RepositoryURL,
				Reference: &v1.GitRepositoryRef{
					Branch: "main",
				},
			},
		}

		err := r.Create(ctx, &gitRepository)
		if err != nil {
			logger.Error(err, "error creating git repository")

			return ctrl.Result{}, err
		}
	}

	var kustomization kustomizev1.Kustomization
	err = r.Get(ctx, req.NamespacedName, &kustomization)
	if client.IgnoreNotFound(err) != nil {
		logger.Error(err, "error getting kustomization")

		return ctrl.Result{}, err
	}

	if apierrors.IsNotFound(err) {
		logger.Info("kustomization not found")

		kustomization = kustomizev1.Kustomization{
			ObjectMeta: metav1.ObjectMeta{
				Name:      containerImageDeployment.Name,
				Namespace: containerImageDeployment.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: dockyardsv1alpha1.GroupVersion.String(),
						Kind:       dockyardsv1alpha1.ContainerImageDeploymentKind,
						Name:       containerImageDeployment.Name,
						UID:        containerImageDeployment.UID,
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
				KubeConfig: &fluxcdmeta.KubeConfigReference{
					SecretRef: fluxcdmeta.SecretKeyReference{
						Name: ownerCluster.Name + "-kubeconfig",
					},
				},
			},
		}

		err := r.Create(ctx, &kustomization)
		if err != nil {
			logger.Error(err, "error creating kustomization")

			return ctrl.Result{}, err
		}

		logger.Info("created kustomization")
	}

	if ownerDeployment.Spec.DeploymentRef.Name == "" {
		logger.Info("owner deployment reference empty")

		patch := client.MergeFrom(ownerDeployment.DeepCopy())

		ownerDeployment.Spec.DeploymentRef = dockyardsv1alpha1.DeploymentReference{
			APIVersion: dockyardsv1alpha1.GroupVersion.String(),
			Kind:       dockyardsv1alpha1.ContainerImageDeploymentKind,
			Name:       containerImageDeployment.Name,
			UID:        containerImageDeployment.UID,
		}

		err := r.Patch(ctx, ownerDeployment, patch)
		if err != nil {
			logger.Error(err, "error patching owner")

			return ctrl.Result{}, err
		}

		logger.Info("patched owner deployment")
	}

	kustomizationReadyCondition := meta.FindStatusCondition(kustomization.Status.Conditions, fluxcdmeta.ReadyCondition)
	if kustomizationReadyCondition == nil {
		logger.Info("kustomization has no ready condition")

		return ctrl.Result{}, nil
	}

	if !meta.IsStatusConditionPresentAndEqual(ownerDeployment.Status.Conditions, dockyardsv1alpha1.ReadyCondition, kustomizationReadyCondition.Status) {
		logger.Info("owner deployment needs status condition update")

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
			logger.Error(err, "error patching owner deployment")

			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *ContainerImageDeploymentReconciler) SetupWithManager(manager ctrl.Manager) error {
	scheme := manager.GetScheme()
	dockyardsv1alpha1.AddToScheme(scheme)
	v1.AddToScheme(scheme)
	kustomizev1.AddToScheme(scheme)

	err := ctrl.NewControllerManagedBy(manager).
		For(&dockyardsv1alpha1.ContainerImageDeployment{}).
		Owns(&v1.GitRepository{}).
		Complete(r)
	if err != nil {
		return err
	}

	return nil
}
