package controller

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"bitbucket.org/sudosweden/dockyards-backend/pkg/api/apiutil"
	dockyardsv1alpha1 "bitbucket.org/sudosweden/dockyards-backend/pkg/api/v1alpha1"
	"github.com/fluxcd/helm-controller/api/v2beta1"
	fluxcdmeta "github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/source-controller/api/v1beta2"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// +kubebuilder:rbac:groups=dockyards.io,resources=clusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=dockyards.io,resources=deployments,verbs=get;list;patch;watch
// +kubebuilder:rbac:groups=dockyards.io,resources=deployments/status,verbs=patch
// +kubebuilder:rbac:groups=dockyards.io,resources=helmdeployments,verbs=get;list;watch
// +kubebuilder:rbac:groups=helm.toolkit.fluxcd.io,resources=helmreleases,verbs=create;get;list;patch;watch
// +kubebuilder:rbac:groups=source.toolkit.fluxcd.io,resources=helmrepositories,verbs=create;get;list;watch

type HelmDeploymentReconciler struct {
	client.Client
	Logger *slog.Logger
}

func (r *HelmDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Logger.With("name", req.Name, "namespace", req.Namespace)

	var helmDeployment dockyardsv1alpha1.HelmDeployment
	err := r.Get(ctx, req.NamespacedName, &helmDeployment)
	if client.IgnoreNotFound(err) != nil {
		logger.Error("error getting helm deployment", "err", err)

		return ctrl.Result{}, err
	}

	if !helmDeployment.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	logger.Debug("reconcile helm deployment")

	ownerDeployment, err := GetOwnerDeployment(ctx, r.Client, &helmDeployment)
	if err != nil {
		logger.Error("error getting owner deployment", "err", err)

		return ctrl.Result{}, err
	}

	if ownerDeployment == nil {
		logger.Info("ignoring helm deployment without owner deployment", "name", helmDeployment.Name)

		return ctrl.Result{}, nil
	}

	ownerCluster, err := apiutil.GetOwnerCluster(ctx, r.Client, ownerDeployment)
	if err != nil {
		logger.Error("error getting owner cluster", "err", err)

		return ctrl.Result{}, err
	}

	if ownerCluster == nil {
		logger.Info("ignoring deployment without owner cluster", "name", ownerDeployment.Name)

		return ctrl.Result{}, nil
	}

	controller := true

	helmRepository := v1beta2.HelmRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      helmDeployment.Name,
			Namespace: helmDeployment.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: dockyardsv1alpha1.GroupVersion.String(),
					Kind:       dockyardsv1alpha1.HelmDeploymentKind,
					Name:       helmDeployment.Name,
					UID:        helmDeployment.UID,
					Controller: &controller,
				},
			},
		},
	}

	operationResult, err := controllerutil.CreateOrPatch(ctx, r.Client, &helmRepository, func() error {
		helmRepository.Spec.URL = helmDeployment.Spec.Repository

		return nil
	})
	if err != nil {
		logger.Error("error reconciling helm repository", "err", err)

		return ctrl.Result{}, err
	}

	logger.Debug("reconciled helm repository", "result", operationResult)

	helmRelease := v2beta1.HelmRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      helmDeployment.Name,
			Namespace: helmDeployment.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: dockyardsv1alpha1.GroupVersion.String(),
					Kind:       dockyardsv1alpha1.HelmDeploymentKind,
					Name:       helmDeployment.Name,
					UID:        helmDeployment.UID,
					Controller: &controller,
				},
			},
		},
	}

	operationResult, err = controllerutil.CreateOrPatch(ctx, r.Client, &helmRelease, func() error {
		helmRelease.Spec.Chart = v2beta1.HelmChartTemplate{
			Spec: v2beta1.HelmChartTemplateSpec{
				Chart:   helmDeployment.Spec.Chart,
				Version: helmDeployment.Spec.Version,
				SourceRef: v2beta1.CrossNamespaceObjectReference{
					APIVersion: v1beta2.GroupVersion.String(),
					Kind:       v1beta2.HelmRepositoryKind,
					Name:       helmRepository.Name,
				},
			},
		}
		helmRelease.Spec.Values = helmDeployment.Spec.Values
		helmRelease.Spec.KubeConfig = &fluxcdmeta.KubeConfigReference{
			SecretRef: fluxcdmeta.SecretKeyReference{
				Name: ownerCluster.Name + "-kubeconfig",
			},
		}
		helmRelease.Spec.TargetNamespace = ownerDeployment.Spec.TargetNamespace
		helmRelease.Spec.StorageNamespace = ownerDeployment.Spec.TargetNamespace
		helmRelease.Spec.Install = &v2beta1.Install{
			CreateNamespace: true,
			Remediation: &v2beta1.InstallRemediation{
				Retries: -1,
			},
		}
		helmRelease.Spec.Interval = metav1.Duration{Duration: time.Minute * 5}
		helmRelease.Spec.ReleaseName = strings.TrimPrefix(helmDeployment.Name, ownerCluster.Name+"-")

		return nil
	})
	if err != nil {
		logger.Error("error reconciling helm release", "err", err)

		return ctrl.Result{}, err
	}

	logger.Debug("reconciled helm release", "result", operationResult)

	if ownerDeployment.Spec.DeploymentRef.Name == "" {
		logger.Debug("owner deployment reference empty")

		patch := client.MergeFrom(ownerDeployment.DeepCopy())

		ownerDeployment.Spec.DeploymentRef = dockyardsv1alpha1.DeploymentReference{
			APIVersion: dockyardsv1alpha1.GroupVersion.String(),
			Kind:       dockyardsv1alpha1.HelmDeploymentKind,
			Name:       helmDeployment.Name,
			UID:        helmDeployment.UID,
		}

		err := r.Patch(ctx, ownerDeployment, patch)
		if err != nil {
			logger.Error("error patching owner deployment")

			return ctrl.Result{}, err
		}
	}

	helmReadyCondition := meta.FindStatusCondition(helmRelease.Status.Conditions, fluxcdmeta.ReadyCondition)
	if helmReadyCondition == nil {
		logger.Debug("helm release has no ready condition")

		return ctrl.Result{}, nil
	}

	if !meta.IsStatusConditionPresentAndEqual(ownerDeployment.Status.Conditions, dockyardsv1alpha1.ReadyCondition, helmReadyCondition.Status) {
		logger.Debug("owner deployment needs status condition update")

		readyCondition := metav1.Condition{
			Type:    dockyardsv1alpha1.ReadyCondition,
			Status:  helmReadyCondition.Status,
			Message: helmReadyCondition.Message,
			Reason:  helmReadyCondition.Reason,
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

func GetOwnerDeployment(ctx context.Context, r client.Client, object client.Object) (*dockyardsv1alpha1.Deployment, error) {
	for _, ownerReference := range object.GetOwnerReferences() {
		if ownerReference.Kind != dockyardsv1alpha1.DeploymentKind {
			continue
		}

		objectKey := client.ObjectKey{
			Name:      ownerReference.Name,
			Namespace: object.GetNamespace(),
		}

		var deployment dockyardsv1alpha1.Deployment
		err := r.Get(ctx, objectKey, &deployment)
		if err != nil {
			return nil, err
		}

		return &deployment, nil
	}

	return nil, nil
}

func (r *HelmDeploymentReconciler) SetupWithManager(manager ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(manager).
		For(&dockyardsv1alpha1.HelmDeployment{}).
		Owns(&v2beta1.HelmRelease{}).
		Owns(&v1beta2.HelmRepository{}).
		Complete(r)
}
