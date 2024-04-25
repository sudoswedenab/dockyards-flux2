package controllers

import (
	"context"
	"strings"
	"time"

	"bitbucket.org/sudosweden/dockyards-backend/pkg/api/apiutil"
	dockyardsv1 "bitbucket.org/sudosweden/dockyards-backend/pkg/api/v1alpha1"
	helmv2 "github.com/fluxcd/helm-controller/api/v2beta2"
	fluxcdmeta "github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	sourcev1 "github.com/fluxcd/source-controller/api/v1beta2"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

// +kubebuilder:rbac:groups=dockyards.io,resources=clusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=dockyards.io,resources=deployments,verbs=get;list;patch;watch
// +kubebuilder:rbac:groups=dockyards.io,resources=deployments/status,verbs=patch
// +kubebuilder:rbac:groups=dockyards.io,resources=helmdeployments,verbs=get;list;watch
// +kubebuilder:rbac:groups=helm.toolkit.fluxcd.io,resources=helmreleases,verbs=create;get;list;patch;watch
// +kubebuilder:rbac:groups=source.toolkit.fluxcd.io,resources=helmrepositories,verbs=create;get;list;patch;watch

type HelmDeploymentReconciler struct {
	client.Client
}

func (r *HelmDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	var helmDeployment dockyardsv1.HelmDeployment
	err := r.Get(ctx, req.NamespacedName, &helmDeployment)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !helmDeployment.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	logger.Info("reconcile helm deployment")

	ownerDeployment, err := GetOwnerDeployment(ctx, r.Client, &helmDeployment)
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

	if conditions.IsFalse(ownerCluster, dockyardsv1.ReadyCondition) {
		logger.Info("ignoring deployment until owner cluster is ready")

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

	logger.Info("reconciled helm repository", "result", operationResult)

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

		helmRelease.Spec.Chart = helmv2.HelmChartTemplate{
			Spec: helmv2.HelmChartTemplateSpec{
				Chart:   helmDeployment.Spec.Chart,
				Version: helmDeployment.Spec.Version,
				SourceRef: helmv2.CrossNamespaceObjectReference{
					APIVersion: sourcev1.GroupVersion.String(),
					Kind:       sourcev1.HelmRepositoryKind,
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

	logger.Info("reconciled helm release", "result", operationResult)

	if ownerDeployment.Spec.DeploymentRef.Name == "" {
		logger.Info("owner deployment reference empty")

		patch := client.MergeFrom(ownerDeployment.DeepCopy())

		ownerDeployment.Spec.DeploymentRef = dockyardsv1.DeploymentReference{
			APIVersion: dockyardsv1.GroupVersion.String(),
			Kind:       dockyardsv1.HelmDeploymentKind,
			Name:       helmDeployment.Name,
			UID:        helmDeployment.UID,
		}

		err := r.Patch(ctx, ownerDeployment, patch)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	helmReadyCondition := meta.FindStatusCondition(helmRelease.Status.Conditions, fluxcdmeta.ReadyCondition)
	if helmReadyCondition == nil {
		logger.Info("helm release has no ready condition")

		return ctrl.Result{}, nil
	}

	if !meta.IsStatusConditionPresentAndEqual(ownerDeployment.Status.Conditions, dockyardsv1.ReadyCondition, helmReadyCondition.Status) {
		logger.Info("owner deployment needs status condition update")

		readyCondition := metav1.Condition{
			Type:    dockyardsv1.ReadyCondition,
			Status:  helmReadyCondition.Status,
			Message: helmReadyCondition.Message,
			Reason:  helmReadyCondition.Reason,
		}

		patch := client.MergeFrom(ownerDeployment.DeepCopy())

		meta.SetStatusCondition(&ownerDeployment.Status.Conditions, readyCondition)

		err := r.Status().Patch(ctx, ownerDeployment, patch)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func GetOwnerDeployment(ctx context.Context, r client.Client, object client.Object) (*dockyardsv1.Deployment, error) {
	for _, ownerReference := range object.GetOwnerReferences() {
		if ownerReference.Kind != dockyardsv1.DeploymentKind {
			continue
		}

		objectKey := client.ObjectKey{
			Name:      ownerReference.Name,
			Namespace: object.GetNamespace(),
		}

		var deployment dockyardsv1.Deployment
		err := r.Get(ctx, objectKey, &deployment)
		if err != nil {
			return nil, err
		}

		return &deployment, nil
	}

	return nil, nil
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
