package controllers

import (
	"context"
	"time"

	"bitbucket.org/sudosweden/dockyards-backend/pkg/api/apiutil"
	dockyardsv1 "bitbucket.org/sudosweden/dockyards-backend/pkg/api/v1alpha2"
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	fluxcdmeta "github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/fluxcd/pkg/runtime/patch"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
// +kubebuilder:rbac:groups=dockyards.io,resources=deployments,verbs=get;list;patch;watch
// +kubebuilder:rbac:groups=dockyards.io,resources=deployments/status,verbs=patch
// +kubebuilder:rbac:groups=dockyards.io,resources=kustomizedeployments,verbs=get;list;patch;watch
// +kubebuilder:rbac:groups=kustomize.toolkit.fluxcd.io,resources=kustomizations,verbs=create;get;list;patch;watch
// +kubebuilder:rbac:groups=source.toolkit.fluxcd.io,resources=gitrepositories,verbs=create;get;list;patch;watch

const (
	KustomizeDeploymentFinalizer = "flux2.dockyards.io/finalizer"
)

type KustomizeDeploymentReconciler struct {
	client.Client
}

func (r *KustomizeDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, reterr error) {
	logger := ctrl.LoggerFrom(ctx)

	var kustomizeDeployment dockyardsv1.KustomizeDeployment
	err := r.Get(ctx, req.NamespacedName, &kustomizeDeployment)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("reconcile kustomize deployment")

	ownerDeployment, err := apiutil.GetOwnerDeployment(ctx, r.Client, &kustomizeDeployment)
	if err != nil {
		return ctrl.Result{}, err
	}

	if ownerDeployment == nil {
		logger.Info("ignoring kustomize deployment without owner deployment")

		return ctrl.Result{}, nil
	}

	ownerCluster, err := apiutil.GetOwnerCluster(ctx, r.Client, ownerDeployment)
	if err != nil {
		return ctrl.Result{}, err
	}

	if ownerCluster == nil {
		logger.Info("ignoring kustomize deployment without owner cluster")

		return ctrl.Result{}, nil
	}

	patchHelper, err := patch.NewHelper(&kustomizeDeployment, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	defer func() {
		err := patchHelper.Patch(ctx, &kustomizeDeployment)
		if err != nil {
			result = ctrl.Result{}
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}
	}()

	if !kustomizeDeployment.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &kustomizeDeployment)
	}

	if kustomizeDeployment.Status.RepositoryURL == "" {
		logger.Info("ignoring kustomize deployment without repository url")

		return ctrl.Result{}, nil
	}

	if !ownerDeployment.Spec.ClusterComponent && conditions.IsFalse(ownerCluster, dockyardsv1.ReadyCondition) {
		logger.Info("ignoring kustomize deployment until cluster is ready")

		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(&kustomizeDeployment, KustomizeDeploymentFinalizer) {
		controllerutil.AddFinalizer(&kustomizeDeployment, KustomizeDeploymentFinalizer)

		return ctrl.Result{}, nil
	}

	gitRepository := sourcev1.GitRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kustomizeDeployment.Name,
			Namespace: kustomizeDeployment.Namespace,
		},
	}

	operationResult, err := controllerutil.CreateOrPatch(ctx, r.Client, &gitRepository, func() error {
		controller := true

		gitRepository.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion:         dockyardsv1.GroupVersion.String(),
				Kind:               dockyardsv1.KustomizeDeploymentKind,
				Name:               kustomizeDeployment.Name,
				UID:                kustomizeDeployment.UID,
				Controller:         &controller,
				BlockOwnerDeletion: &controller,
			},
		}

		gitRepository.Spec.Interval = metav1.Duration{Duration: time.Minute * 5}
		gitRepository.Spec.URL = kustomizeDeployment.Status.RepositoryURL

		gitRepository.Spec.Reference = &sourcev1.GitRepositoryRef{
			Branch: "main",
		}

		return nil
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("reconciled git repository", "result", operationResult)

	kustomization := kustomizev1.Kustomization{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kustomizeDeployment.Name,
			Namespace: kustomizeDeployment.Namespace,
		},
	}

	operationResult, err = controllerutil.CreateOrPatch(ctx, r.Client, &kustomization, func() error {
		controller := true

		kustomization.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion:         dockyardsv1.GroupVersion.String(),
				Kind:               dockyardsv1.KustomizeDeploymentKind,
				Name:               kustomizeDeployment.Name,
				UID:                kustomizeDeployment.UID,
				Controller:         &controller,
				BlockOwnerDeletion: &controller,
			},
		}

		kustomization.Spec.Interval = metav1.Duration{Duration: time.Minute * 10}
		kustomization.Spec.TargetNamespace = ownerDeployment.Spec.TargetNamespace

		kustomization.Spec.SourceRef = kustomizev1.CrossNamespaceSourceReference{
			APIVersion: sourcev1.GroupVersion.String(),
			Kind:       sourcev1.GitRepositoryKind,
			Name:       gitRepository.Name,
		}

		kustomization.Spec.Prune = true
		kustomization.Spec.Timeout = &metav1.Duration{Duration: time.Minute}

		kustomization.Spec.KubeConfig = &fluxcdmeta.KubeConfigReference{
			SecretRef: fluxcdmeta.SecretKeyReference{
				Name: ownerCluster.Name + "-kubeconfig",
			},
		}

		kustomization.Spec.RetryInterval = &metav1.Duration{Duration: time.Minute}

		return nil
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("reconciled kustomization", "result", operationResult)

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

func (r *KustomizeDeploymentReconciler) reconcileDelete(ctx context.Context, kustomizeDeployment *dockyardsv1.KustomizeDeployment) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	var kustomization kustomizev1.Kustomization
	err := r.Get(ctx, client.ObjectKeyFromObject(kustomizeDeployment), &kustomization)
	if client.IgnoreNotFound(err) != nil {
		return ctrl.Result{}, err
	}

	if apierrors.IsNotFound(err) {
		controllerutil.RemoveFinalizer(kustomizeDeployment, KustomizeDeploymentFinalizer)

		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(&kustomization, kustomizev1.KustomizationFinalizer) {
		controllerutil.RemoveFinalizer(kustomizeDeployment, KustomizeDeploymentFinalizer)

		return ctrl.Result{}, nil
	}

	objectKey := client.ObjectKey{
		Name:      kustomization.Spec.KubeConfig.SecretRef.Name,
		Namespace: kustomization.Namespace,
	}

	var secret corev1.Secret
	err = r.Get(ctx, objectKey, &secret)
	if client.IgnoreNotFound(err) != nil {
		return ctrl.Result{}, err
	}

	if !apierrors.IsNotFound(err) {
		logger.Info("ignoring kustomization with kubeconfig secret")

		return ctrl.Result{RequeueAfter: time.Second * 15}, nil
	}

	patch := client.MergeFrom(kustomization.DeepCopy())

	if controllerutil.RemoveFinalizer(&kustomization, kustomizev1.KustomizationFinalizer) {
		err := r.Patch(ctx, &kustomization, patch)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	controllerutil.RemoveFinalizer(kustomizeDeployment, KustomizeDeploymentFinalizer)

	return ctrl.Result{}, nil
}

func (r *KustomizeDeploymentReconciler) DockyardsClusterToKustomizeDeployments(ctx context.Context, o client.Object) []ctrl.Request {
	logger := ctrl.LoggerFrom(ctx)

	dockyardsCluster, ok := o.(*dockyardsv1.Cluster)
	if !ok {
		return nil
	}

	matchingLabels := client.MatchingLabels{
		dockyardsv1.LabelClusterName: dockyardsCluster.Name,
	}

	var kustomizeDeploymentList dockyardsv1.KustomizeDeploymentList
	err := r.List(ctx, &kustomizeDeploymentList, matchingLabels, client.InNamespace(dockyardsCluster.Namespace))
	if err != nil {
		logger.Error(err, "error listing kustomize deployments")

		return nil
	}

	requests := make([]ctrl.Request, len(kustomizeDeploymentList.Items))
	for i, kustomizeDeployment := range kustomizeDeploymentList.Items {
		requests[i] = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      kustomizeDeployment.Name,
				Namespace: kustomizeDeployment.Namespace,
			},
		}
	}

	return requests
}

func (r *KustomizeDeploymentReconciler) SetupWithManager(m ctrl.Manager) error {
	scheme := m.GetScheme()

	_ = dockyardsv1.AddToScheme(scheme)
	_ = kustomizev1.AddToScheme(scheme)
	_ = sourcev1.AddToScheme(scheme)

	return ctrl.NewControllerManagedBy(m).
		For(&dockyardsv1.KustomizeDeployment{}).
		Owns(&sourcev1.GitRepository{}).
		Owns(&kustomizev1.Kustomization{}).
		Watches(
			&dockyardsv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(r.DockyardsClusterToKustomizeDeployments),
		).
		Complete(r)
}
