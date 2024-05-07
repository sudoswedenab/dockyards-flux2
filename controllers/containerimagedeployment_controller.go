package controllers

import (
	"context"
	"time"

	"bitbucket.org/sudosweden/dockyards-backend/pkg/api/apiutil"
	dockyardsv1 "bitbucket.org/sudosweden/dockyards-backend/pkg/api/v1alpha2"
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	fluxcdmeta "github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

// +kubebuilder:rbac:groups=dockyards.io,resources=clusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=dockyards.io,resources=containerimagedeployments,verbs=get;list;watch
// +kubebuilder:rbac:groups=dockyards.io,resources=deployments,verbs=get;list;patch;watch
// +kubebuilder:rbac:groups=dockyards.io,resources=deployments/status,verbs=patch
// +kubebuilder:rbac:groups=kustomize.toolkit.fluxcd.io,resources=kustomizations,verbs=create;get;list;patch;watch
// +kubebuilder:rbac:groups=source.toolkit.fluxcd.io,resources=gitrepositories,verbs=create;get;list;patch;watch

type ContainerImageDeploymentReconciler struct {
	client.Client
}

func (r *ContainerImageDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	var containerImageDeployment dockyardsv1.ContainerImageDeployment
	err := r.Get(ctx, req.NamespacedName, &containerImageDeployment)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !containerImageDeployment.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	ownerDeployment, err := apiutil.GetOwnerDeployment(ctx, r.Client, &containerImageDeployment)
	if err != nil {
		return ctrl.Result{}, err
	}

	if ownerDeployment == nil {
		logger.Info("ignoring container image deployment without owner")

		return ctrl.Result{}, nil
	}

	ownerCluster, err := apiutil.GetOwnerCluster(ctx, r.Client, ownerDeployment)
	if err != nil {
		return ctrl.Result{}, err
	}

	if ownerCluster == nil {
		logger.Info("ignoring container image deployment without cluster")

		return ctrl.Result{}, nil
	}

	if conditions.IsFalse(ownerCluster, dockyardsv1.ReadyCondition) {
		logger.Info("ignoring container image deployment until cluster is ready")

		return ctrl.Result{}, nil
	}

	if containerImageDeployment.Status.RepositoryURL == "" {
		logger.Info("ignoring container image deployment without repository url")

		return ctrl.Result{}, nil
	}

	gitRepository := sourcev1.GitRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      containerImageDeployment.Name,
			Namespace: containerImageDeployment.Namespace,
		},
	}

	operationResult, err := controllerutil.CreateOrPatch(ctx, r.Client, &gitRepository, func() error {
		controller := true

		gitRepository.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion:         dockyardsv1.GroupVersion.String(),
				Kind:               dockyardsv1.ContainerImageDeploymentKind,
				Name:               containerImageDeployment.Name,
				UID:                containerImageDeployment.UID,
				Controller:         &controller,
				BlockOwnerDeletion: &controller,
			},
		}

		gitRepository.Spec.Interval = metav1.Duration{
			Duration: time.Minute * 5,
		}

		gitRepository.Spec.URL = containerImageDeployment.Status.RepositoryURL

		gitRepository.Spec.Reference = &sourcev1.GitRepositoryRef{
			Branch: "main",
		}

		return nil
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	if operationResult != controllerutil.OperationResultNone {
		logger.Info("reconciled git repository", "result", operationResult)
	}

	kustomization := kustomizev1.Kustomization{
		ObjectMeta: metav1.ObjectMeta{
			Name:      containerImageDeployment.Name,
			Namespace: containerImageDeployment.Namespace,
		},
	}

	operationResult, err = controllerutil.CreateOrPatch(ctx, r.Client, &kustomization, func() error {
		controller := true

		kustomization.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion:         dockyardsv1.GroupVersion.String(),
				Kind:               dockyardsv1.ContainerImageDeploymentKind,
				Name:               containerImageDeployment.Name,
				UID:                containerImageDeployment.UID,
				Controller:         &controller,
				BlockOwnerDeletion: &controller,
			},
		}

		kustomization.Spec.Interval = metav1.Duration{
			Duration: time.Minute * 10,
		}

		kustomization.Spec.TargetNamespace = ownerDeployment.Spec.TargetNamespace

		kustomization.Spec.SourceRef = kustomizev1.CrossNamespaceSourceReference{
			APIVersion: sourcev1.GroupVersion.String(),
			Kind:       sourcev1.GitRepositoryKind,
			Name:       gitRepository.Name,
		}

		kustomization.Spec.Prune = true

		kustomization.Spec.Timeout = &metav1.Duration{
			Duration: time.Minute,
		}

		kustomization.Spec.KubeConfig = &fluxcdmeta.KubeConfigReference{
			SecretRef: fluxcdmeta.SecretKeyReference{
				Name: ownerCluster.Name + "-kubeconfig",
			},
		}

		return nil
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	if operationResult != controllerutil.OperationResultNone {
		logger.Info("reconciled kustomization", "result", operationResult)
	}

	kustomizationReadyCondition := meta.FindStatusCondition(kustomization.Status.Conditions, fluxcdmeta.ReadyCondition)
	if kustomizationReadyCondition == nil {
		logger.Info("kustomization has no ready condition")

		return ctrl.Result{}, nil
	}

	if !meta.IsStatusConditionPresentAndEqual(ownerDeployment.Status.Conditions, dockyardsv1.ReadyCondition, kustomizationReadyCondition.Status) {
		logger.Info("owner deployment needs status condition update")

		readyCondition := metav1.Condition{
			Type:    dockyardsv1.ReadyCondition,
			Status:  kustomizationReadyCondition.Status,
			Message: kustomizationReadyCondition.Message,
			Reason:  kustomizationReadyCondition.Reason,
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

func (r *ContainerImageDeploymentReconciler) DockyardsClusterToContainerImageDeployments(ctx context.Context, o client.Object) []ctrl.Request {
	logger := ctrl.LoggerFrom(ctx)

	dockyardsCluster, ok := o.(*dockyardsv1.Cluster)
	if !ok {
		return nil
	}

	matchingLabels := client.MatchingLabels{
		dockyardsv1.LabelClusterName: dockyardsCluster.Name,
	}

	var containerImageDeploymentList dockyardsv1.ContainerImageDeploymentList
	err := r.List(ctx, &containerImageDeploymentList, matchingLabels, client.InNamespace(dockyardsCluster.Namespace))
	if err != nil {
		logger.Error(err, "error listing container image deployments")

		return nil
	}

	requests := make([]ctrl.Request, len(containerImageDeploymentList.Items))
	for i, containerImageDeployment := range containerImageDeploymentList.Items {
		requests[i] = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      containerImageDeployment.Name,
				Namespace: containerImageDeployment.Namespace,
			},
		}
	}

	return requests
}

func (r *ContainerImageDeploymentReconciler) SetupWithManager(manager ctrl.Manager) error {
	scheme := manager.GetScheme()

	_ = dockyardsv1.AddToScheme(scheme)
	_ = sourcev1.AddToScheme(scheme)
	_ = kustomizev1.AddToScheme(scheme)

	err := ctrl.NewControllerManagedBy(manager).
		For(&dockyardsv1.ContainerImageDeployment{}).
		Owns(&sourcev1.GitRepository{}).
		Owns(&kustomizev1.Kustomization{}).
		Watches(
			&dockyardsv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(r.DockyardsClusterToContainerImageDeployments),
		).
		Complete(r)
	if err != nil {
		return err
	}

	return nil
}
