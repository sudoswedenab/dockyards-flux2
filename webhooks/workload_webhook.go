package webhooks

import (
	"context"
	"os"
	"path"

	dockyardsv1 "bitbucket.org/sudosweden/dockyards-backend/pkg/api/v1alpha3"
	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	cueerrors "cuelang.org/go/cue/errors"
	"cuelang.org/go/cue/load"
	cuejson "cuelang.org/go/encoding/json"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:rbac:groups=dockyards.io,resources=workloadtemplates,verbs=get;list;watch
// +kubebuilder:webhook:groups=dockyards.io,resources=workloads,verbs=create;update,path=/validate-dockyards-io-v1alpha3-workload,mutating=false,failurePolicy=fail,sideEffects=none,admissionReviewVersions=v1,versions=v1alpha3,name=validation.workload.dockyards.io

type DockyardsWorkload struct {
	Client client.Reader
}

var _ webhook.CustomValidator = &DockyardsWorkload{}

func (webhook *DockyardsWorkload) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&dockyardsv1.Workload{}).WithValidator(webhook).Complete()
}

func (webhook *DockyardsWorkload) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	workload, ok := obj.(*dockyardsv1.Workload)
	if !ok {
		return nil, apierrors.NewBadRequest("unexpected type")
	}

	return webhook.validate(ctx, workload)
}

func (webhook *DockyardsWorkload) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (webhook *DockyardsWorkload) ValidateUpdate(ctx context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	workload, ok := newObj.(*dockyardsv1.Workload)
	if !ok {
		return nil, apierrors.NewBadRequest("unexpected type")
	}

	return webhook.validate(ctx, workload)
}

func (webhook *DockyardsWorkload) validate(ctx context.Context, workload *dockyardsv1.Workload) (admission.Warnings, error) {
	var allWarnings admission.Warnings
	var allErrors field.ErrorList

	if workload.Spec.WorkloadTemplateInput != nil { //nolint:staticcheck
		allWarnings = append(allWarnings, "ignoring deprecated field workloadTemplateInput")
	}

	if workload.Spec.WorkloadTemplateRef == nil {
		return allWarnings, nil
	}

	objectKey := client.ObjectKey{
		Name:      workload.Spec.WorkloadTemplateRef.Name,
		Namespace: *workload.Spec.WorkloadTemplateRef.Namespace,
	}

	var workloadTemplate dockyardsv1.WorkloadTemplate
	err := webhook.Client.Get(ctx, objectKey, &workloadTemplate)
	if client.IgnoreNotFound(err) != nil {
		internalError := field.InternalError(field.NewPath("spec", "workloadTemplateRef"), err)

		allErrors = append(allErrors, internalError)

		return allWarnings, apierrors.NewInvalid(dockyardsv1.GroupVersion.WithKind(dockyardsv1.WorkloadKind).GroupKind(), workload.Name, allErrors)
	}

	if apierrors.IsNotFound(err) {
		notFound := field.NotFound(field.NewPath("spec", "workloadTemplateRef"), workload.Spec.WorkloadTemplateRef.Name)

		allErrors = append(allErrors, notFound)

		return allWarnings, apierrors.NewInvalid(dockyardsv1.GroupVersion.WithKind(dockyardsv1.WorkloadKind).GroupKind(), workload.Name, allErrors)
	}

	source := load.FromString(workloadTemplate.Spec.Source)

	cuectx := cuecontext.New()

	wd, err := os.Getwd()
	if err != nil {
		return allWarnings, err
	}

	filename := path.Join(wd, "template.cue")

	instances := load.Instances([]string{}, &load.Config{
		Package: "template",
		Overlay: map[string]load.Source{
			filename: source,
		},
	})

	instance := instances[0]

	value := cuectx.BuildInstance(instance)
	if value.Err() != nil {
		return allWarnings, err
	}

	input := value.LookupPath(cue.MakePath(cue.Def("#Input")))
	if !input.Exists() && workload.Spec.Input != nil {
		forbidden := field.Forbidden(field.NewPath("spec", "input"), "input not supported on template")

		allErrors = append(allErrors, forbidden)

		return allWarnings, apierrors.NewInvalid(dockyardsv1.GroupVersion.WithKind(dockyardsv1.WorkloadKind).GroupKind(), workload.Name, allErrors)
	}

	if !input.Exists() {
		return allWarnings, nil
	}

	raw := []byte("{}")
	if workload.Spec.Input != nil {
		raw = workload.Spec.Input.Raw
	}

	err = cuejson.Validate(raw, input)
	if err != nil {
		cueerrs := cueerrors.Errors(err)

		for _, cueerr := range cueerrs {
			invalid := field.Forbidden(field.NewPath("spec", "input"), cueerr.Error())

			allErrors = append(allErrors, invalid)
		}

		return allWarnings, apierrors.NewInvalid(dockyardsv1.GroupVersion.WithKind(dockyardsv1.WorkloadKind).GroupKind(), workload.Name, allErrors)
	}

	return allWarnings, nil
}
