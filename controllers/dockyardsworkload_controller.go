package controllers

import (
	"context"
	"encoding/base64"
	"os"
	"path"

	"bitbucket.org/sudosweden/dockyards-backend/pkg/api/apiutil"
	dockyardsv1 "bitbucket.org/sudosweden/dockyards-backend/pkg/api/v1alpha3"
	"bitbucket.org/sudosweden/dockyards-backend/pkg/api/v1alpha3/index"
	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/errors"
	"cuelang.org/go/cue/load"
	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/fluxcd/pkg/runtime/patch"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

// +kubebuilder:rbac:groups=dockyards.io,resources=workloads/status,verbs=patch
// +kubebuilder:rbac:groups=dockyards.io,resources=workloads,verbs=get;list;watch
// +kubebuilder:rbac:groups=dockyards.io,resources=workloadtemplates,verbs=get;list;watch
// +kubebuilder:rbac:groups=dockyards.io,resources=worktrees,verbs=create;get;list;patch;watch
// +kubebuilder:rbac:groups=helm.toolkit.fluxcd.io,resources=*,verbs=create;get;list;patch;watch
// +kubebuilder:rbac:groups=kustomize.toolkit.fluxcd.io,resources=*,verbs=create;get;list;patch;watch
// +kubebuilder:rbac:groups=source.toolkit.fluxcd.io,resources=*,verbs=create;get;list;patch;watch

type DockyardsWorkloadReconciler struct {
	client.Client
}

func (r *DockyardsWorkloadReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, reterr error) {
	var workload dockyardsv1.Workload
	err := r.Get(ctx, req.NamespacedName, &workload)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	patchHelper, err := patch.NewHelper(&workload, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	defer func() {
		summaryConditions := []string{
			dockyardsv1.WorkloadTemplateReconciledCondition,
			dockyardsv1.ReconcilingCondition,
		}

		negativePolarityConditions := []string{
			dockyardsv1.ReconcilingCondition,
		}

		conditions.SetSummary(
			&workload,
			dockyardsv1.ReadyCondition,
			conditions.WithConditions(summaryConditions...),
			conditions.WithNegativePolarityConditions(negativePolarityConditions...),
		)

		err := patchHelper.Patch(ctx, &workload)
		if err != nil {
			result = ctrl.Result{}
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}
	}()

	result, err = r.reconcileWorkloadTemplate(ctx, &workload)
	if err != nil {
		return result, err
	}

	result, err = r.reconcileReconcilingCondition(ctx, &workload)
	if err != nil {
		return result, err
	}

	return ctrl.Result{}, nil
}

func (r *DockyardsWorkloadReconciler) reconcileWorkloadTemplate(ctx context.Context, workload *dockyardsv1.Workload) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	if !workload.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	ownerCluster, err := apiutil.GetOwnerCluster(ctx, r.Client, workload)
	if err != nil {
		return ctrl.Result{}, err
	}

	if ownerCluster == nil {
		logger.Info("ignoring workload without cluster owner")

		return ctrl.Result{}, nil
	}

	if workload.Spec.WorkloadTemplateRef == nil {
		logger.Info("ignoring workload with empty template reference")

		return ctrl.Result{}, nil
	}

	objectKey := client.ObjectKey{
		Name:      workload.Spec.WorkloadTemplateRef.Name,
		Namespace: workload.Namespace,
	}

	if workload.Spec.WorkloadTemplateRef.Namespace != nil {
		objectKey.Namespace = *workload.Spec.WorkloadTemplateRef.Namespace
	}

	var workloadTemplate dockyardsv1.WorkloadTemplate
	err = r.Get(ctx, objectKey, &workloadTemplate)
	if client.IgnoreNotFound(err) != nil {
		return ctrl.Result{}, err
	}

	if apierrors.IsNotFound(err) {
		conditions.MarkFalse(workload, dockyardsv1.WorkloadTemplateReconciledCondition, WaitingForWorkloadTemplateReason, "")

		return ctrl.Result{}, nil
	}

	if workloadTemplate.Spec.Type != dockyardsv1.WorkloadTemplateTypeCue {
		conditions.MarkFalse(workload, dockyardsv1.WorkloadTemplateReconciledCondition, IncorrectWorkloadTemplateTypeReason, "")

		return ctrl.Result{}, nil
	}

	if !workload.Spec.ClusterComponent && conditions.IsFalse(ownerCluster, dockyardsv1.ReadyCondition) {
		conditions.MarkFalse(workload, dockyardsv1.WorkloadTemplateReconciledCondition, WaitingForClusterReadyReason, "")

		return ctrl.Result{}, nil
	}

	source := load.FromString(workloadTemplate.Spec.Source)

	cuectx := cuecontext.New()

	wd, err := os.Getwd()
	if err != nil {
		return ctrl.Result{}, err
	}

	filename := path.Join(wd, "template.cue")

	instances := load.Instances([]string{}, &load.Config{
		Package: "template",
		Overlay: map[string]load.Source{
			filename: source,
		},
	})

	if len(instances) != 1 {
		conditions.MarkFalse(workload, dockyardsv1.WorkloadTemplateReconciledCondition, LoadInstanceFailedReason, "incorrect instance count %d", len(instances))

		return ctrl.Result{}, nil
	}

	instance := instances[0]

	value := cuectx.BuildInstance(instance)
	if value.Err() != nil {
		conditions.MarkFalse(workload, dockyardsv1.WorkloadTemplateReconciledCondition, BuildInstanceFailedReason, "%s", err)

		return ctrl.Result{}, nil
	}

	lookup := value.LookupPath(cue.MakePath(cue.Def("#workload")))
	if lookup.Err() != nil {
		conditions.MarkFalse(workload, dockyardsv1.WorkloadTemplateReconciledCondition, LookupPathFailedReason, "%s", lookup.Err())

		return ctrl.Result{}, nil
	}

	lookup = value.LookupPath(cue.MakePath(cue.Def("#cluster")))
	if lookup.Err() != nil {
		conditions.MarkFalse(workload, dockyardsv1.WorkloadTemplateReconciledCondition, LookupPathFailedReason, "%s", lookup.Err())

		return ctrl.Result{}, nil
	}

	v := value.FillPath(cue.MakePath(cue.Def("#cluster")), ownerCluster)
	if v.Err() != nil {
		conditions.MarkFalse(workload, dockyardsv1.WorkloadTemplateReconciledCondition, FillPathFailedReason, "%s", v.Err())

		return ctrl.Result{}, nil
	}

	v = v.FillPath(cue.MakePath(cue.Def("#workload")), &workload)
	if v.Err() != nil {
		conditions.MarkFalse(workload, dockyardsv1.WorkloadTemplateReconciledCondition, FillPathFailedReason, "%s", v.Err())

		return ctrl.Result{}, nil
	}

	err = v.Validate()
	if err != nil {
		conditions.MarkFalse(workload, dockyardsv1.WorkloadTemplateReconciledCondition, ValidateFailedReason, "%s", err)

		return ctrl.Result{}, nil
	}

	iterator, err := v.Fields()
	if err != nil {
		conditions.MarkFalse(workload, dockyardsv1.WorkloadTemplateReconciledCondition, FieldsFailedReason, "%s", err)

		return ctrl.Result{}, nil
	}

	for iterator.Next() {
		value := iterator.Value()

		err := value.Validate(cue.Concrete(true))
		if err != nil {
			conditions.MarkFalse(workload, dockyardsv1.WorkloadTemplateReconciledCondition, ValidateFailedReason, "%s", err)

			cueerrors := errors.Errors(err)
			for _, cueerr := range cueerrors {
				logger.Error(cueerr, "cue error validating field")
			}

			return ctrl.Result{}, nil
		}

		var u unstructured.Unstructured
		err = value.Decode(&u)
		if err != nil {
			conditions.MarkFalse(workload, dockyardsv1.WorkloadTemplateReconciledCondition, DecodeFailedReason, "%s", err)

			return ctrl.Result{}, nil
		}

		operationResult, err := controllerutil.CreateOrPatch(ctx, r.Client, &u, func() error {
			references := []metav1.OwnerReference{
				{
					APIVersion:         dockyardsv1.GroupVersion.String(),
					Kind:               dockyardsv1.WorkloadKind,
					Name:               workload.Name,
					UID:                workload.UID,
					BlockOwnerDeletion: ptr.To(true),
					Controller:         ptr.To(true),
				},
			}

			u.SetOwnerReferences(references)

			labels := map[string]string{
				dockyardsv1.LabelWorkloadName: workload.Name,
			}

			u.SetLabels(labels)

			spec := value.LookupPath(cue.ParsePath("spec"))
			if !spec.Exists() {
				return nil
			}

			nestedMap := make(map[string]any)

			err = spec.Decode(&nestedMap)
			if err != nil {
				return err
			}

			ensureValidJSON(nestedMap)

			err = unstructured.SetNestedMap(u.Object, nestedMap, "spec")
			if err != nil {
				return err
			}

			return nil
		})
		if err != nil {
			conditions.MarkFalse(workload, dockyardsv1.WorkloadTemplateReconciledCondition, ReconcileFailedReason, "%s", err)

			return ctrl.Result{}, nil
		}

		if operationResult != controllerutil.OperationResultNone {
			logger.Info("reconciled unstructured", "name", u.GetName(), "result", operationResult)
		}
	}

	conditions.MarkTrue(workload, dockyardsv1.WorkloadTemplateReconciledCondition, dockyardsv1.ReadyReason, "")

	return ctrl.Result{}, nil
}

func (r *DockyardsWorkloadReconciler) reconcileReconcilingCondition(ctx context.Context, workload *dockyardsv1.Workload) (ctrl.Result, error) {
	matchingLabels := client.MatchingLabels{
		dockyardsv1.LabelWorkloadName: workload.Name,
	}

	inNamespace := client.InNamespace(workload.Namespace)

	var kustomizationList kustomizev1.KustomizationList
	err := r.List(ctx, &kustomizationList, matchingLabels, inNamespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	for _, kustomization := range kustomizationList.Items {
		if conditions.IsReconciling(&kustomization) {
			conditions.MarkTrue(workload, dockyardsv1.ReconcilingCondition, KustomizationReconcilingReason, "")

			return ctrl.Result{}, nil
		}
	}

	var helmReleaseList helmv2.HelmReleaseList
	err = r.List(ctx, &helmReleaseList, matchingLabels, inNamespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	for _, helmRelease := range helmReleaseList.Items {
		if conditions.IsReconciling(&helmRelease) {
			conditions.MarkTrue(workload, dockyardsv1.ReconcilingCondition, HelmReleaseReconcilingReason, "")

			return ctrl.Result{}, nil
		}
	}

	conditions.Delete(workload, dockyardsv1.ReconcilingCondition)

	return ctrl.Result{}, nil
}

func (r *DockyardsWorkloadReconciler) workloadTemplateToWorkloads(ctx context.Context, obj client.Object) []ctrl.Request {
	workloadTemplate, ok := obj.(*dockyardsv1.WorkloadTemplate)
	if !ok {
		return nil
	}

	ref := corev1.TypedObjectReference{
		Kind:      dockyardsv1.WorkloadTemplateKind,
		Name:      workloadTemplate.Name,
		Namespace: &workloadTemplate.Namespace,
	}

	matchingFields := client.MatchingFields{
		index.WorkloadTemplateReferenceField: index.TypedObjectRef(&ref),
	}

	var workloadList dockyardsv1.WorkloadList
	err := r.List(ctx, &workloadList, matchingFields)
	if err != nil {
		panic(err)
	}

	result := []ctrl.Request{}

	for _, workload := range workloadList.Items {
		result = append(result, ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      workload.Name,
				Namespace: workload.Namespace,
			},
		})
	}

	return result
}

func (r *DockyardsWorkloadReconciler) clusterToWorkloads(ctx context.Context, obj client.Object) []ctrl.Request {
	cluster, ok := obj.(*dockyardsv1.Cluster)
	if !ok {
		return nil
	}

	matchingLabels := client.MatchingLabels{
		dockyardsv1.LabelClusterName: cluster.Name,
	}

	var workloadList dockyardsv1.WorkloadList
	err := r.List(ctx, &workloadList, matchingLabels, client.InNamespace(cluster.Namespace))
	if err != nil {
		panic(err)
	}

	result := []ctrl.Request{}

	for _, workload := range workloadList.Items {
		result = append(result, ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      workload.Name,
				Namespace: workload.Namespace,
			},
		})
	}

	return result
}

func ensureValidJSON(x map[string]any) {
	for k, v := range x {
		switch t := v.(type) {
		case map[string]any:
			ensureValidJSON(t)
		case int32:
			x[k] = int64(t)
		case int:
			x[k] = int64(t)
		case []byte:
			x[k] = base64.StdEncoding.EncodeToString(t)
		}
	}
}

func (r *DockyardsWorkloadReconciler) SetupWithManager(mgr ctrl.Manager) error {
	scheme := mgr.GetScheme()

	_ = dockyardsv1.AddToScheme(scheme)
	_ = helmv2.AddToScheme(scheme)
	_ = kustomizev1.AddToScheme(scheme)

	err := ctrl.NewControllerManagedBy(mgr).
		For(&dockyardsv1.Workload{}).
		Owns(&kustomizev1.Kustomization{}).
		Owns(&helmv2.HelmRelease{}).
		Watches(
			&dockyardsv1.WorkloadTemplate{},
			handler.EnqueueRequestsFromMapFunc(r.workloadTemplateToWorkloads),
		).
		Watches(
			&dockyardsv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(r.clusterToWorkloads),
		).
		Complete(r)
	if err != nil {
		return err
	}

	return nil
}
