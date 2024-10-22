package controllers

import (
	"context"
	"slices"

	"bitbucket.org/sudosweden/dockyards-backend/pkg/api/apiutil"
	dockyardsv1 "bitbucket.org/sudosweden/dockyards-backend/pkg/api/v1alpha3"
	"bitbucket.org/sudosweden/dockyards-flux2/pkg/ingressutil"
	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/controllers/remote"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// +kubebuilder:rbac:groups=dockyards.io,resources=deployments,verbs=get;list;watch
// +kubebuilder:rbac:groups=dockyards.io,resources=deployments/status,verbs=patch
// +kubebuilder:rbac:groups=dockyards.io,resources=helmdeployments,verbs=get;list;watch
// +kubebuilder:rbac:groups=dockyards.io,resources=clusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch

var (
	HelmOriginLabelNameKey      = helmv2.GroupVersion.Group + "/name"
	HelmOriginLabelNamespaceKey = helmv2.GroupVersion.Group + "/namespace"
)

type HelmReleaseReconciler struct {
	client.Client
	Tracker    *remote.ClusterCacheTracker
	controller controller.Controller
}

func (r *HelmReleaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	var helmRelease helmv2.HelmRelease
	err := r.Get(ctx, req.NamespacedName, &helmRelease)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !helmRelease.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	logger.Info("reconcile helm release")

	ownerHelmDeployment, err := apiutil.GetOwnerHelmDeployment(ctx, r.Client, &helmRelease)
	if err != nil {
		return ctrl.Result{}, err
	}

	if ownerHelmDeployment == nil {
		logger.Info("ignoring helm release without owner helm deployment")

		return ctrl.Result{}, nil
	}

	ownerDeployment, err := apiutil.GetOwnerDeployment(ctx, r.Client, ownerHelmDeployment)
	if err != nil {
		return ctrl.Result{}, err
	}

	if ownerDeployment == nil {
		logger.Info("ignoring helm release without owner deployent")

		return ctrl.Result{}, nil
	}

	ownerCluster, err := apiutil.GetOwnerCluster(ctx, r.Client, ownerDeployment)
	if err != nil {
		return ctrl.Result{}, err
	}

	if ownerCluster == nil {
		logger.Info("ignoring deployment without owner cluster")

		return ctrl.Result{}, nil
	}

	err = r.watchClusterIngresses(ctx, ownerCluster)
	if err != nil {
		logger.Error(err, "error watching remote cluster ingresses")

		return ctrl.Result{}, nil
	}

	namespacedName := types.NamespacedName{
		Name:      ownerCluster.Name,
		Namespace: ownerCluster.Namespace,
	}

	trackerClient, err := r.Tracker.GetClient(ctx, namespacedName)
	if err != nil {
		return ctrl.Result{}, err
	}

	var ingressList networkingv1.IngressList

	matchingLabels := client.MatchingLabels{
		HelmOriginLabelNameKey:      helmRelease.Name,
		HelmOriginLabelNamespaceKey: helmRelease.Namespace,
	}

	err = trackerClient.List(ctx, &ingressList, matchingLabels)
	if err != nil {
		return ctrl.Result{}, err
	}

	urls := []string{}

	for _, ingress := range ingressList.Items {
		urls = append(urls, ingressutil.GetURLsFromIngress(&ingress)...)

		logger.Info("ingress", "name", ingress.Name, "urls", urls)
	}

	if !slices.Equal(ownerDeployment.Status.URLs, urls) {
		logger.Info("urls needs update")

		patch := client.MergeFrom(ownerDeployment.DeepCopy())

		ownerDeployment.Status.URLs = urls

		err := r.Status().Patch(ctx, ownerDeployment, patch)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *HelmReleaseReconciler) watchClusterIngresses(ctx context.Context, cluster *dockyardsv1.Cluster) error {
	if r.Tracker == nil {
		return nil
	}

	namespacedName := types.NamespacedName{
		Name:      cluster.Name,
		Namespace: cluster.Namespace,
	}

	watchInput := remote.WatchInput{
		Name:         "deployment-ingress",
		Cluster:      namespacedName,
		Watcher:      r.controller,
		Kind:         &networkingv1.Ingress{},
		EventHandler: handler.EnqueueRequestsFromMapFunc(r.ingressToHelmRelease),
	}

	err := r.Tracker.Watch(ctx, watchInput)
	if err != nil {
		return err
	}

	return nil
}

func (r *HelmReleaseReconciler) ingressToHelmRelease(_ context.Context, o client.Object) []reconcile.Request {
	ingress, ok := o.(*networkingv1.Ingress)
	if !ok {
		return nil
	}

	originLabelName, hasLabel := ingress.Labels[HelmOriginLabelNameKey]
	if !hasLabel {
		return nil
	}

	originLabelNamespace, hasLabel := ingress.Labels[HelmOriginLabelNamespaceKey]
	if !hasLabel {
		return nil
	}

	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Name:      originLabelName,
				Namespace: originLabelNamespace,
			},
		},
	}
}

func (r *HelmReleaseReconciler) SetupWithManager(m ctrl.Manager) error {
	scheme := m.GetScheme()

	_ = helmv2.AddToScheme(scheme)
	_ = dockyardsv1.AddToScheme(scheme)

	c, err := ctrl.NewControllerManagedBy(m).For(&helmv2.HelmRelease{}).Build(r)
	if err != nil {
		return err
	}

	r.controller = c

	return nil
}
