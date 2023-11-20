package controller

import (
	"context"
	"log/slog"

	"bitbucket.org/sudosweden/dockyards-backend/pkg/api/apiutil"
	"bitbucket.org/sudosweden/dockyards-backend/pkg/api/v1alpha1"
	"github.com/fluxcd/helm-controller/api/v2beta1"
	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/source-controller/api/v1beta2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// +kubebuilder:rbac:groups=dockyards.io,resources=clusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=dockyards.io,resources=deployments,verbs=get;list;watch
// +kubebuilder:rbac:groups=dockyards.io,resources=helmdeployments,verbs=get;list;watch
// +kubebuilder:rbac:groups=helm.toolkit.fluxcd.io,resources=helmreleases,verbs=create;get;list;watch
// +kubebuilder:rbac:groups=source.toolkit.fluxcd.io,resources=helmrepositories,verbs=create;get;list;watch

type HelmDeploymentReconciler struct {
	client.Client
	Logger *slog.Logger
}

func (r *HelmDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var helmDeployment v1alpha1.HelmDeployment
	err := r.Get(ctx, req.NamespacedName, &helmDeployment)
	if client.IgnoreNotFound(err) != nil {
		r.Logger.Error("error getting helm deployment", "err", err)

		return ctrl.Result{}, err
	}

	r.Logger.Debug("reconcile helm deployment", "name", helmDeployment.Name)

	ownerDeployment, err := GetOwnerDeployment(ctx, r.Client, &helmDeployment)
	if err != nil {
		r.Logger.Error("error getting owner deployment", "err", err)

		return ctrl.Result{}, err
	}

	if ownerDeployment == nil {
		r.Logger.Info("ignoring helm deployment without owner deployment", "name", helmDeployment.Name)

		return ctrl.Result{}, nil
	}

	ownerCluster, err := apiutil.GetOwnerCluster(ctx, r.Client, ownerDeployment)
	if err != nil {
		r.Logger.Error("error getting owner cluster", "err", err)

		return ctrl.Result{}, err
	}

	if ownerCluster == nil {
		r.Logger.Info("ignoring deployment without owner cluster", "name", ownerDeployment.Name)

		return ctrl.Result{}, nil
	}

	var helmRepository v1beta2.HelmRepository
	err = r.Get(ctx, req.NamespacedName, &helmRepository)
	if client.IgnoreNotFound(err) != nil {
		r.Logger.Error("error getting helm repository", "err", err)

		return ctrl.Result{}, err
	}

	if apierrors.IsNotFound(err) {
		helmRepository = v1beta2.HelmRepository{
			ObjectMeta: metav1.ObjectMeta{
				Name:      helmDeployment.Name,
				Namespace: helmDeployment.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: v1alpha1.GroupVersion.String(),
						Kind:       v1alpha1.HelmDeploymentKind,
						Name:       helmDeployment.Name,
						UID:        helmDeployment.UID,
					},
				},
			},
			Spec: v1beta2.HelmRepositorySpec{
				URL: helmDeployment.Spec.Repository,
			},
		}

		err := r.Create(ctx, &helmRepository)
		if err != nil {
			r.Logger.Error("error creating helm repository", "err", err)

			return ctrl.Result{}, err
		}
	}

	var helmRelease v2beta1.HelmRelease
	err = r.Get(ctx, req.NamespacedName, &helmRelease)
	if client.IgnoreNotFound(err) != nil {
		r.Logger.Error("error getting helm release", "err", err)

		return ctrl.Result{}, err
	}

	if apierrors.IsNotFound(err) {
		r.Logger.Info("helm release not found")

		helmRelease := v2beta1.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      helmDeployment.Name,
				Namespace: helmDeployment.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: v1alpha1.GroupVersion.String(),
						Kind:       v1alpha1.HelmDeploymentKind,
						Name:       helmDeployment.Name,
						UID:        helmDeployment.UID,
					},
				},
			},
			Spec: v2beta1.HelmReleaseSpec{
				Chart: v2beta1.HelmChartTemplate{
					Spec: v2beta1.HelmChartTemplateSpec{
						Chart:   helmDeployment.Spec.Chart,
						Version: helmDeployment.Spec.Version,
						SourceRef: v2beta1.CrossNamespaceObjectReference{
							APIVersion: v1beta2.GroupVersion.String(),
							Kind:       v1beta2.HelmRepositoryKind,
							Name:       helmRepository.Name,
						},
					},
				},
				Values: helmDeployment.Spec.Values,
				KubeConfig: &meta.KubeConfigReference{
					SecretRef: meta.SecretKeyReference{
						Name: ownerCluster.Name + "-kubeconfig",
					},
				},
				TargetNamespace:  ownerDeployment.Spec.TargetNamespace,
				StorageNamespace: ownerDeployment.Spec.TargetNamespace,
				Install: &v2beta1.Install{
					CreateNamespace: true,
				},
			},
		}

		err := r.Create(ctx, &helmRelease)
		if err != nil {
			r.Logger.Error("error creating helm release", "err", err)

			return ctrl.Result{}, err
		}
	}

	if ownerDeployment.Spec.DeploymentRef.Name == "" {
		r.Logger.Debug("owner deployment reference empty")

		patch := client.MergeFrom(ownerDeployment.DeepCopy())

		ownerDeployment.Spec.DeploymentRef = v1alpha1.DeploymentReference{
			APIVersion: v1alpha1.GroupVersion.String(),
			Kind:       v1alpha1.KustomizeDeploymentKind,
			Name:       helmDeployment.Name,
			UID:        helmDeployment.UID,
		}

		err := r.Patch(ctx, ownerDeployment, patch)
		if err != nil {
			r.Logger.Error("error patching owner deployment")

			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func GetOwnerDeployment(ctx context.Context, r client.Client, object client.Object) (*v1alpha1.Deployment, error) {
	for _, ownerReference := range object.GetOwnerReferences() {
		if ownerReference.Kind != v1alpha1.DeploymentKind {
			continue
		}

		objectKey := client.ObjectKey{
			Name:      ownerReference.Name,
			Namespace: object.GetNamespace(),
		}

		var deployment v1alpha1.Deployment
		err := r.Get(ctx, objectKey, &deployment)
		if err != nil {
			return nil, err
		}

		return &deployment, nil
	}

	return nil, nil
}

func (r *HelmDeploymentReconciler) SetupWithManager(manager ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(manager).For(&v1alpha1.HelmDeployment{}).Complete(r)
}
