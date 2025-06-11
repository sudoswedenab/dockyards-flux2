// Copyright 2025 Sudo Sweden AB
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package webhooks

import (
	"bufio"
	"context"
	"io"
	"os"
	"path"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	cueerrors "cuelang.org/go/cue/errors"
	"cuelang.org/go/cue/load"
	dockyardsv1 "github.com/sudoswedenab/dockyards-backend/api/v1alpha3"
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

	reader := strings.NewReader(workloadTemplate.Spec.Source)

	for _, instance := range instances {
		if instance.Err != nil {
			position := instance.Err.Position()

			_, err := reader.Seek(int64(position.Offset()), io.SeekStart)
			if err != nil {
				return err
			}

			scanner := bufio.NewScanner(reader)
			scanner.Scan()

			invalid := field.Invalid(
				field.NewPath("spec", "source"),
				scanner.Text(),
				instance.Err.Error(),
			)

			errorList = append(errorList, invalid)

			break
		}

		v := cuectx.BuildInstance(instance)
		if v.Err() != nil {
			cueerrs := cueerrors.Errors(v.Err())

			for _, cueerr := range cueerrs {
				position := cueerr.Position()

				_, err := reader.Seek(int64(position.Offset()), io.SeekStart)
				if err != nil {
					return err
				}

				scanner := bufio.NewScanner(reader)
				scanner.Scan()

				invalid := field.Invalid(
					field.NewPath("spec", "source"),
					scanner.Text(),
					cueerr.Error(),
				)

				errorList = append(errorList, invalid)
			}

			break
		}

		x := v.LookupPath(cue.MakePath(cue.Def("cluster")))
		if !x.Exists() {
			notFound := field.Required(
				field.NewPath("spec", "source").Child("#cluster"),
				"must have cluster definition",
			)

			errorList = append(errorList, notFound)
		}

		x = v.LookupPath(cue.MakePath(cue.Def("workload")))
		if !x.Exists() {
			notFound := field.Required(
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
