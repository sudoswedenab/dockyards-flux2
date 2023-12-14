package controller

import (
	"context"
	"slices"

	"bitbucket.org/sudosweden/dockyards-backend/pkg/api/apiutil"
	dockyardsv1 "bitbucket.org/sudosweden/dockyards-backend/pkg/api/v1alpha1"
	"bitbucket.org/sudosweden/dockyards-flux2/pkg/dockyardsutil"
	"bitbucket.org/sudosweden/dockyards-flux2/pkg/ingressutil"
	"github.com/fluxcd/helm-controller/api/v2beta1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/controllers/remote"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	HelmOriginLabelNameKey      = v2beta1.GroupVersion.Group + "/name"
	HelmOriginLabelNamespaceKey = v2beta1.GroupVersion.Group + "/namespace"
)

type HelmReleaseReconciler struct {
	client.Client
	Tracker    *remote.ClusterCacheTracker
	controller controller.Controller
}

func (r *HelmReleaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	var helmRelease v2beta1.HelmRelease
	err := r.Get(ctx, req.NamespacedName, &helmRelease)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("reconcile helm release")

	ownerHelmDeployment, err := dockyardsutil.GetOwnerHelmDeployment(ctx, r.Client, &helmRelease)
	if err != nil {
		logger.Error(err, "error getting owner helm deployment")

		return ctrl.Result{}, err
	}

	if ownerHelmDeployment == nil {
		logger.Info("ignoring helm release without owner helm deployment")

		return ctrl.Result{}, nil
	}

	ownerDeployment, err := dockyardsutil.GetOwnerDeployment(ctx, r.Client, ownerHelmDeployment)
	if err != nil {
		logger.Error(err, "error getting owner deployment")

		return ctrl.Result{}, err
	}

	if ownerDeployment == nil {
		logger.Info("ignoring helm release without owner deployent")

		return ctrl.Result{}, nil
	}

	ownerCluster, err := apiutil.GetOwnerCluster(ctx, r.Client, ownerDeployment)
	if err != nil {
		logger.Error(err, "error getting owner cluster")

		return ctrl.Result{}, err
	}

	if ownerCluster == nil {
		logger.Info("ignoring deployment without owner cluster")

		return ctrl.Result{}, nil
	}

	namespacedName := types.NamespacedName{
		Name:      ownerCluster.Name,
		Namespace: ownerCluster.Namespace,
	}

	trackerClient, err := r.Tracker.GetClient(ctx, namespacedName)
	if err != nil {
		logger.Error(err, "error getting cluster client")
	}

	var ingressList networkingv1.IngressList

	matchingLabels := client.MatchingLabels{
		HelmOriginLabelNameKey:      helmRelease.Name,
		HelmOriginLabelNamespaceKey: helmRelease.Namespace,
	}

	err = trackerClient.List(ctx, &ingressList, matchingLabels)
	if err != nil {
		logger.Error(err, "error listing cluster ingresses")

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
			logger.Error(err, "error patching deployment")

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

func (r *HelmReleaseReconciler) ingressToHelmRelease(ctx context.Context, o client.Object) []reconcile.Request {
	ingress, ok := o.(*networkingv1.Ingress)
	if !ok {
		panic("not ok")
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

	_ = v2beta1.AddToScheme(scheme)
	_ = dockyardsv1.AddToScheme(scheme)

	c, err := ctrl.NewControllerManagedBy(m).For(&v2beta1.HelmRelease{}).Build(r)
	if err != nil {
		return err
	}

	r.controller = c

	return nil
}
