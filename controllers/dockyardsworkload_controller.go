package controllers

import (
	"context"
	"os"
	"path"

	"bitbucket.org/sudosweden/dockyards-backend/pkg/api/apiutil"
	dockyardsv1 "bitbucket.org/sudosweden/dockyards-backend/pkg/api/v1alpha3"
	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/errors"
	"cuelang.org/go/cue/load"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// +kubebuilder:rbac:groups=dockyards.io,resources=workloads,verbs=get;list;watch
// +kubebuilder:rbac:groups=dockyards.io,resources=workloadtemplates,verbs=get;list;watch
// +kubebuilder:rbac:groups=helm.toolkit.fluxcd.io,resources=*,verbs=create;get;list;patch;watch
// +kubebuilder:rbac:groups=kustomize.toolkit.fluxcd.io,resources=*,verbs=create;get;list;patch;watch
// +kubebuilder:rbac:groups=source.toolkit.fluxcd.io,resources=*,verbs=create;get;list;patch;watch

type DockyardsWorkloadReconciler struct {
	client.Client
}

func (r *DockyardsWorkloadReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	var workload dockyardsv1.Workload
	err := r.Get(ctx, req.NamespacedName, &workload)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	ownerCluster, err := apiutil.GetOwnerCluster(ctx, r.Client, &workload)
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
		logger.Info("ignoring workload with missing template reference")

		return ctrl.Result{}, nil
	}

	if workloadTemplate.Spec.Type != dockyardsv1.WorkloadTemplateTypeCue {
		logger.Info("ignoring workload template with unsupported type")

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
		logger.Info("ignoring unexpected instances", "count", len(instances))

		return ctrl.Result{}, nil
	}

	instance := instances[0]

	value := cuectx.BuildInstance(instance)
	if value.Err() != nil {
		logger.Error(err, "error building instance")

		return ctrl.Result{}, value.Err()
	}

	lookup := value.LookupPath(cue.MakePath(cue.Def("#workload")))
	if lookup.Err() != nil {
		logger.Error(lookup.Err(), "lookup path error")

		return ctrl.Result{}, lookup.Err()
	}

	lookup = value.LookupPath(cue.MakePath(cue.Def("#cluster")))
	if lookup.Err() != nil {
		logger.Error(lookup.Err(), "lookup path error")

		return ctrl.Result{}, lookup.Err()
	}

	v := value.FillPath(cue.MakePath(cue.Def("#cluster")), ownerCluster)
	if v.Err() != nil {
		return ctrl.Result{}, v.Err()
	}

	v = v.FillPath(cue.MakePath(cue.Def("#workload")), &workload)
	if v.Err() != nil {
		return ctrl.Result{}, v.Err()
	}

	err = v.Validate()
	if err != nil {
		logger.Error(err, "error validating value")

		return ctrl.Result{}, err
	}

	iterator, err := v.Fields()
	if err != nil {
		return ctrl.Result{}, err
	}

	for iterator.Next() {
		value := iterator.Value()

		err := value.Validate(cue.Concrete(true))
		if err != nil {
			logger.Error(err, "error validating final value")

			cueerrors := errors.Errors(err)
			for _, cueerr := range cueerrors {
				logger.Error(cueerr, "cue error")
			}

			return ctrl.Result{}, err
		}

		var u unstructured.Unstructured
		err = value.Decode(&u)
		if err != nil {
			logger.Error(err, "error decoding value into unstructured", "value", value)

			return ctrl.Result{}, err
		}

		operationResult, err := controllerutil.CreateOrPatch(ctx, r.Client, &u, func() error {
			references := []metav1.OwnerReference{
				{
					APIVersion: dockyardsv1.GroupVersion.String(),
					Kind:       dockyardsv1.WorkloadKind,
					Name:       workload.Name,
					UID:        workload.UID,
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

			ensureInt64(nestedMap)

			err = unstructured.SetNestedMap(u.Object, nestedMap, "spec")
			if err != nil {
				return err
			}

			return nil
		})
		if err != nil {
			return ctrl.Result{}, err
		}

		if operationResult != controllerutil.OperationResultNone {
			logger.Info("reconciled unstructured", "name", u.GetName(), "result", operationResult)
		}
	}

	return ctrl.Result{}, nil
}

func ensureInt64(x map[string]any) {
	for k, v := range x {
		switch t := v.(type) {
		case map[string]any:
			ensureInt64(t)
		case int32:
			x[k] = int64(t)
		case int:
			x[k] = int64(t)
		}
	}
}

func (r *DockyardsWorkloadReconciler) SetupWithManager(mgr ctrl.Manager) error {
	scheme := mgr.GetScheme()

	_ = dockyardsv1.AddToScheme(scheme)

	err := ctrl.NewControllerManagedBy(mgr).For(&dockyardsv1.Workload{}).Complete(r)
	if err != nil {
		return err
	}

	return nil
}
