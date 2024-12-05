package controllers

import (
	"context"
	"strings"
	"time"

	"bitbucket.org/sudosweden/dockyards-backend/pkg/api/apiutil"
	dockyardsv1 "bitbucket.org/sudosweden/dockyards-backend/pkg/api/v1alpha3"
	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	fluxcdmeta "github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/fluxcd/pkg/runtime/patch"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

// +kubebuilder:rbac:groups=dockyards.io,resources=clusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=dockyards.io,resources=deployments,verbs=get;list;watch
// +kubebuilder:rbac:groups=dockyards.io,resources=helmdeployments,verbs=get;list;watch
// +kubebuilder:rbac:groups=dockyards.io,resources=helmdeployments/status,verbs=patch
// +kubebuilder:rbac:groups=helm.toolkit.fluxcd.io,resources=helmreleases,verbs=create;get;list;patch;watch
// +kubebuilder:rbac:groups=source.toolkit.fluxcd.io,resources=helmrepositories,verbs=create;get;list;patch;watch

type HelmDeploymentReconciler struct {
	client.Client
}

func (r *HelmDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, reterr error) {
	logger := ctrl.LoggerFrom(ctx)

	var helmDeployment dockyardsv1.HelmDeployment
	err := r.Get(ctx, req.NamespacedName, &helmDeployment)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	patchHelper, err := patch.NewHelper(&helmDeployment, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	defer func() {
		err := patchHelmDeployment(ctx, patchHelper, &helmDeployment)
		if err != nil {
			result = ctrl.Result{}
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}
	}()

	if !helmDeployment.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	ownerDeployment, err := apiutil.GetOwnerDeployment(ctx, r.Client, &helmDeployment)
	if err != nil {
		return ctrl.Result{}, err
	}

	if ownerDeployment == nil {
		logger.Info("ignoring helm deployment without owner deployment", "name", helmDeployment.Name)

		return ctrl.Result{}, nil
	}

	ownerCluster, err := apiutil.GetOwnerCluster(ctx, r.Client, ownerDeployment)
	if err != nil {
		return ctrl.Result{}, err
	}

	if ownerCluster == nil {
		logger.Info("ignoring deployment without owner cluster", "name", ownerDeployment.Name)

		return ctrl.Result{}, nil
	}

	if !ownerDeployment.Spec.ClusterComponent && conditions.IsFalse(ownerCluster, dockyardsv1.ReadyCondition) {
		conditions.MarkFalse(&helmDeployment, HelmReleaseReadyCondition, WaitingForClusterReadyReason, "")

		return ctrl.Result{}, nil
	}

	helmRepository := sourcev1.HelmRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      helmDeployment.Name,
			Namespace: helmDeployment.Namespace,
		},
	}

	operationResult, err := controllerutil.CreateOrPatch(ctx, r.Client, &helmRepository, func() error {
		controller := true

		helmRepository.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion:         dockyardsv1.GroupVersion.String(),
				Kind:               dockyardsv1.HelmDeploymentKind,
				Name:               helmDeployment.Name,
				UID:                helmDeployment.UID,
				Controller:         &controller,
				BlockOwnerDeletion: &controller,
			},
		}

		helmRepository.Spec.URL = helmDeployment.Spec.Repository

		return nil
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	if operationResult != controllerutil.OperationResultNone {
		logger.Info("reconciled helm repository", "result", operationResult)
	}

	helmRelease := helmv2.HelmRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      helmDeployment.Name,
			Namespace: helmDeployment.Namespace,
		},
	}

	operationResult, err = controllerutil.CreateOrPatch(ctx, r.Client, &helmRelease, func() error {
		controller := true

		helmRelease.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion:         dockyardsv1.GroupVersion.String(),
				Kind:               dockyardsv1.HelmDeploymentKind,
				Name:               helmDeployment.Name,
				UID:                helmDeployment.UID,
				Controller:         &controller,
				BlockOwnerDeletion: &controller,
			},
		}

		helmRelease.Spec.Chart = &helmv2.HelmChartTemplate{
			Spec: helmv2.HelmChartTemplateSpec{
				Chart:   helmDeployment.Spec.Chart,
				Version: helmDeployment.Spec.Version,
				SourceRef: helmv2.CrossNamespaceObjectReference{
					APIVersion: sourcev1.GroupVersion.String(),
					Kind:       sourcev1.HelmRepositoryKind,
					Name:       helmRepository.Name,
				},
			},
			ObjectMeta: &helmv2.HelmChartTemplateObjectMeta{
				Labels: map[string]string{
					dockyardsv1.LabelClusterName:    ownerCluster.Name,
					dockyardsv1.LabelDeploymentName: ownerDeployment.Name,
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

		helmRelease.Spec.Install = &helmv2.Install{
			CreateNamespace: !helmDeployment.Spec.SkipNamespace,
			Remediation: &helmv2.InstallRemediation{
				Retries: -1,
			},
		}

		helmRelease.Spec.Interval = metav1.Duration{Duration: time.Minute * 5}
		helmRelease.Spec.ReleaseName = strings.TrimPrefix(helmDeployment.Name, ownerCluster.Name+"-")

		return nil
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	if operationResult != controllerutil.OperationResultNone {
		logger.Info("reconciled helm release", "result", operationResult)
	}

	if operationResult == controllerutil.OperationResultCreated {
		return ctrl.Result{}, nil
	}

	helmReleaseReadyCondition := meta.FindStatusCondition(helmRelease.Status.Conditions, fluxcdmeta.ReadyCondition)
	if helmReleaseReadyCondition == nil {
		conditions.MarkFalse(&helmDeployment, HelmReleaseReadyCondition, WaitingForHelmReleaseConditionReason, "")

		return ctrl.Result{}, nil
	}

	condition := metav1.Condition{
		Type:               HelmReleaseReadyCondition,
		Reason:             helmReleaseReadyCondition.Reason,
		Status:             helmReleaseReadyCondition.Status,
		Message:            helmReleaseReadyCondition.Message,
		LastTransitionTime: helmReleaseReadyCondition.LastTransitionTime,
	}

	conditions.Set(&helmDeployment, &condition)

	return ctrl.Result{}, nil
}

func (r *HelmDeploymentReconciler) DockyardsClusterToHelmDeployments(ctx context.Context, o client.Object) []ctrl.Request {
	logger := ctrl.LoggerFrom(ctx)

	dockyardsCluster, ok := o.(*dockyardsv1.Cluster)
	if !ok {
		return nil
	}

	matchingLabels := client.MatchingLabels{
		dockyardsv1.LabelClusterName: dockyardsCluster.Name,
	}

	var helmDeploymentList dockyardsv1.HelmDeploymentList
	err := r.List(ctx, &helmDeploymentList, matchingLabels, client.InNamespace(dockyardsCluster.Namespace))
	if err != nil {
		logger.Error(err, "error listing helm deployments")

		return nil
	}

	requests := make([]ctrl.Request, len(helmDeploymentList.Items))
	for i, helmDeployment := range helmDeploymentList.Items {
		requests[i] = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      helmDeployment.Name,
				Namespace: helmDeployment.Namespace,
			},
		}
	}

	return requests
}

func (r *HelmDeploymentReconciler) SetupWithManager(m ctrl.Manager) error {
	scheme := m.GetScheme()

	_ = dockyardsv1.AddToScheme(scheme)
	_ = helmv2.AddToScheme(scheme)
	_ = sourcev1.AddToScheme(scheme)

	return ctrl.NewControllerManagedBy(m).
		For(&dockyardsv1.HelmDeployment{}).
		Owns(&helmv2.HelmRelease{}).
		Owns(&sourcev1.HelmRepository{}).
		Watches(
			&dockyardsv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(r.DockyardsClusterToHelmDeployments),
		).
		Complete(r)
}

func patchHelmDeployment(ctx context.Context, patchHelper *patch.Helper, helmDeployment *dockyardsv1.HelmDeployment) error {
	summaryConditions := []string{
		HelmReleaseReadyCondition,
	}

	conditions.SetSummary(helmDeployment, dockyardsv1.ReadyCondition, conditions.WithConditions(summaryConditions...))

	return patchHelper.Patch(ctx, helmDeployment)
}
