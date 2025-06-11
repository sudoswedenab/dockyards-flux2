package controllers

import (
	"context"
	"time"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	dockyardsv1 "github.com/sudoswedenab/dockyards-backend/api/v1alpha3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// +kubebuilder:rbac:groups=dockyards.io,resources=worktrees,verbs=get;list;watch
// +kubebuilder:rbac:groups=source.toolkit.fluxcd.io,resources=gitrepositories,verbs=create;get;list;patch;watch

type DockyardsWorktreeReconciler struct {
	client.Client
}

func (r *DockyardsWorktreeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	var dockyardsWorktree dockyardsv1.Worktree
	err := r.Get(ctx, req.NamespacedName, &dockyardsWorktree)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if dockyardsWorktree.Status.URL == nil {
		return ctrl.Result{}, nil
	}

	if dockyardsWorktree.Status.ReferenceName == nil && dockyardsWorktree.Status.CommitHash == nil {
		return ctrl.Result{}, nil
	}

	gitRepository := sourcev1.GitRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dockyardsWorktree.Name,
			Namespace: dockyardsWorktree.Namespace,
		},
	}

	operationResult, err := controllerutil.CreateOrPatch(ctx, r.Client, &gitRepository, func() error {
		gitRepository.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion:         dockyardsv1.GroupVersion.String(),
				Kind:               dockyardsv1.WorktreeKind,
				Name:               dockyardsWorktree.Name,
				UID:                dockyardsWorktree.UID,
				BlockOwnerDeletion: ptr.To(true),
			},
		}

		gitRepository.Spec.URL = *dockyardsWorktree.Status.URL
		gitRepository.Spec.Interval = metav1.Duration{Duration: time.Minute * 5}

		if dockyardsWorktree.Status.ReferenceName != nil {
			gitRepository.Spec.Reference = &sourcev1.GitRepositoryRef{
				Name: *dockyardsWorktree.Status.ReferenceName,
			}
		}

		if dockyardsWorktree.Status.CommitHash != nil {
			gitRepository.Spec.Reference = &sourcev1.GitRepositoryRef{
				Commit: *dockyardsWorktree.Status.CommitHash,
			}
		}

		return nil
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	if operationResult != controllerutil.OperationResultNone {
		logger.Info("reconciled git repository", "result", operationResult)
	}

	return ctrl.Result{}, nil
}

func (r *DockyardsWorktreeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	scheme := mgr.GetScheme()

	_ = dockyardsv1.AddToScheme(scheme)
	_ = sourcev1.AddToScheme(scheme)

	err := ctrl.NewControllerManagedBy(mgr).For(&dockyardsv1.Worktree{}).Complete(r)
	if err != nil {
		return err
	}

	return nil
}
