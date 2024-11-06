package webhooks

import (
	"context"
	"fmt"
	"os"
	"path"

	dockyardsv1 "bitbucket.org/sudosweden/dockyards-backend/pkg/api/v1alpha3"
	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:groups=dockyards.io,resources=workloadtemplates,verbs=create;update,path=/validate-dockyards-io-v1alpha3-workloadtemplate,mutating=false,failurePolicy=fail,sideEffects=none,admissionReviewVersions=v1,versions=v1alpha3,name=validation.workloadtemplate.dockyards.io

type DockyardsWorkloadTemplate struct{}

var _ webhook.CustomValidator = &DockyardsWorkloadTemplate{}

func (webhook *DockyardsWorkloadTemplate) SetupWithManager(mgr ctrl.Manager) error {
	scheme := mgr.GetScheme()

	_ = dockyardsv1.AddToScheme(scheme)

	return ctrl.NewWebhookManagedBy(mgr).For(&dockyardsv1.WorkloadTemplate{}).WithValidator(webhook).Complete()
}

func (webhook *DockyardsWorkloadTemplate) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	workloadTemplate, ok := obj.(*dockyardsv1.WorkloadTemplate)
	if !ok {
		return nil, apierrors.NewBadRequest("unexpected type")
	}

	return nil, webhook.validate(workloadTemplate)
}

func (webhook *DockyardsWorkloadTemplate) ValidateUpdate(_ context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	workloadTemplate, ok := newObj.(*dockyardsv1.WorkloadTemplate)
	if !ok {
		return nil, apierrors.NewBadRequest("unexpected type")
	}

	return nil, webhook.validate(workloadTemplate)
}

func (webhook *DockyardsWorkloadTemplate) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (webhook *DockyardsWorkloadTemplate) validate(workloadTemplate *dockyardsv1.WorkloadTemplate) error {
	source := load.FromString(workloadTemplate.Spec.Source)

	cuectx := cuecontext.New()

	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	filename := path.Join(wd, "template.cue")

	instances := load.Instances([]string{}, &load.Config{
		Package: "template",
		Overlay: map[string]load.Source{
			filename: source,
		},
	})

	var errorList field.ErrorList

	for _, instance := range instances {
		if instance.Err != nil {
			invalid := field.Invalid(
				field.NewPath("spec", "source"),
				workloadTemplate.Spec.Source,
				fmt.Sprintf("err: %s", instance.Err),
			)
			errorList = append(errorList, invalid)

			break
		}

		v := cuectx.BuildInstance(instance)
		if v.Err() != nil {
			invalid := field.Invalid(
				field.NewPath("spec", "source"),
				workloadTemplate.Spec.Source,
				fmt.Sprintf("err: %s", v.Err()),
			)

			errorList = append(errorList, invalid)
		}

		x := v.LookupPath(cue.MakePath(cue.Def("cluster")))
		if !x.Exists() {
			notFound := field.NotFound(
				field.NewPath("spec", "source").Child("#cluster"),
				"must have cluster definition",
			)

			errorList = append(errorList, notFound)
		}

		x = v.LookupPath(cue.MakePath(cue.Def("workload")))
		if !x.Exists() {
			notFound := field.NotFound(
				field.NewPath("spec", "source").Child("#workload"),
				"must have workload definition",
			)

			errorList = append(errorList, notFound)
		}
	}

	if len(errorList) != 0 {
		return apierrors.NewInvalid(dockyardsv1.GroupVersion.WithKind(dockyardsv1.WorkloadTemplateKind).GroupKind(), workloadTemplate.Name, errorList)
	}

	return nil
}
