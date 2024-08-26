package controllers

import (
	"context"

	dockyardsv1 "bitbucket.org/sudosweden/dockyards-backend/pkg/api/v1alpha2"
	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/fluxcd/pkg/runtime/patch"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// +kubebuilder:groups=dockyards.io,resources=containerimagedeployments,verbs=get;list;watch
// +kubebuilder:groups=dockyards.io,resources=deployments,verbs=get;list;watch
// +kubebuilder:groups=dockyards.io,resources=deployments/status,verbs=patch
// +kubebuilder:groups=dockyards.io,resources=helmdeployments,verbs=get;list;watch
// +kubebuilder:groups=dockyards.io,resources=kustomizedeployments,verbs=get;list;watch

type DockyardsDeploymentReconciler struct {
	client.Client
}

func (r *DockyardsDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, reterr error) {
	logger := ctrl.LoggerFrom(ctx)

	var dockyardsDeployment dockyardsv1.Deployment
	err := r.Get(ctx, req.NamespacedName, &dockyardsDeployment)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	patchHelper, err := patch.NewHelper(&dockyardsDeployment, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	defer func() {
		err := patchDockyardsDeployment(ctx, patchHelper, &dockyardsDeployment)
		if err != nil {
			result = ctrl.Result{}
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}
	}()

	for _, deploymentRef := range dockyardsDeployment.Spec.DeploymentRefs {
		logger.Info("deployment reference", "name", deploymentRef.Name, "kind", deploymentRef.Kind)

		objectKey := client.ObjectKey{
			Name:      deploymentRef.Name,
			Namespace: dockyardsDeployment.Namespace,
		}

		var readyCondition *metav1.Condition

		switch deploymentRef.Kind {
		case dockyardsv1.KustomizeDeploymentKind:
			var kustomizeDeployment dockyardsv1.KustomizeDeployment
			err := r.Get(ctx, objectKey, &kustomizeDeployment)
			if err != nil {
				logger.Error(err, "error getting kustomize deployment")
				panic(err)
			}

			readyCondition = conditions.Get(&kustomizeDeployment, dockyardsv1.ReadyCondition)
		case dockyardsv1.HelmDeploymentKind:
			var helmDeployment dockyardsv1.HelmDeployment
			err := r.Get(ctx, objectKey, &helmDeployment)
			if err != nil {
				panic(err)
			}

			readyCondition = conditions.Get(&helmDeployment, dockyardsv1.ReadyCondition)
		case dockyardsv1.ContainerImageDeploymentKind:
			var containerImageDeployment dockyardsv1.ContainerImageDeployment
			err = r.Get(ctx, objectKey, &containerImageDeployment)
			if err != nil {
				panic(err)
			}

			readyCondition = conditions.Get(&containerImageDeployment, dockyardsv1.ReadyCondition)
		default:
			conditions.MarkFalse(&dockyardsDeployment, DeploymentReferencesReadyCondition, UnsupportedDeploymentReferenceKindReason, "reference kind '%s' unsupported", deploymentRef.Kind)

			return ctrl.Result{}, nil
		}

		if readyCondition == nil {
			conditions.MarkFalse(&dockyardsDeployment, DeploymentReferencesReadyCondition, WaitingForDeploymentReadyConditionReason, "reference '%s/%s' has no ready condition", deploymentRef.Kind, deploymentRef.Name)

			return ctrl.Result{}, nil
		}

		if readyCondition.Status != metav1.ConditionTrue {
			conditions.MarkFalse(&dockyardsDeployment, DeploymentReferencesReadyCondition, DeploymentReferenceNotReadyReason, "reference '%s/%s' is not ready", deploymentRef.Kind, deploymentRef.Name)

			return ctrl.Result{}, nil
		}
	}

	conditions.MarkTrue(&dockyardsDeployment, DeploymentReferencesReadyCondition, DeploymentReferencesReadyCondition, "")

	return ctrl.Result{}, nil
}

func (r *DockyardsDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	scheme := mgr.GetScheme()

	_ = dockyardsv1.AddToScheme(scheme)

	err := ctrl.NewControllerManagedBy(mgr).For(&dockyardsv1.Deployment{}).Complete(r)
	if err != nil {
		return err
	}

	return nil
}

func patchDockyardsDeployment(ctx context.Context, patchHelper *patch.Helper, dockyardsDeployment *dockyardsv1.Deployment) error {
	summaryConditions := []string{
		DeploymentReferencesReadyCondition,
	}

	conditions.SetSummary(dockyardsDeployment, dockyardsv1.ReadyCondition, conditions.WithConditions(summaryConditions...))

	return patchHelper.Patch(ctx, dockyardsDeployment)
}
