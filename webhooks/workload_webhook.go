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
	"context"
	"os"
	"path"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	cueerrors "cuelang.org/go/cue/errors"
	"cuelang.org/go/cue/load"
	cuejson "cuelang.org/go/encoding/json"
	dockyardsv1 "github.com/sudoswedenab/dockyards-backend/api/v1alpha3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:rbac:groups=dockyards.io,resources=workloadtemplates,verbs=get;list;watch
// +kubebuilder:rbac:groups=dockyards.io,resources=clusters,verbs=get;list;watch
// +kubebuilder:webhook:groups=dockyards.io,resources=workloads,verbs=create,path=/mutate-dockyards-io-v1alpha3-workload,mutating=true,failurePolicy=fail,sideEffects=none,admissionReviewVersions=v1,versions=v1alpha3,name=default.workload.dockyards.io
// +kubebuilder:webhook:groups=dockyards.io,resources=workloads,verbs=create;update,path=/validate-dockyards-io-v1alpha3-workload,mutating=false,failurePolicy=fail,sideEffects=none,admissionReviewVersions=v1,versions=v1alpha3,name=validation.workload.dockyards.io

type DockyardsWorkload struct {
	Client client.Reader
}

var (
	_ webhook.CustomValidator = &DockyardsWorkload{}
	_ webhook.CustomDefaulter = &DockyardsWorkload{}
)

func (webhook *DockyardsWorkload) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&dockyardsv1.Workload{}).WithDefaulter(webhook).WithValidator(webhook).Complete()
}

func (webhook *DockyardsWorkload) Default(ctx context.Context, obj runtime.Object) error {
	workload, ok := obj.(*dockyardsv1.Workload)
	if !ok {
		return apierrors.NewBadRequest("unexpected type")
	}

	if hasClusterOwnerReference(workload) {
		return nil
	}

	clusterName := workload.Labels[dockyardsv1.LabelClusterName]
	if clusterName == "" {
		return nil
	}

	cluster, err := webhook.getCluster(ctx, workload.Namespace, clusterName)
	if apierrors.IsNotFound(err) {
		return nil
	}

	if err != nil {
		return err
	}

	ownerReferences := workload.GetOwnerReferences()
	ownerReferences = append(ownerReferences, metav1.OwnerReference{
		APIVersion: dockyardsv1.GroupVersion.String(),
		Kind:       dockyardsv1.ClusterKind,
		Name:       cluster.Name,
		UID:        cluster.UID,
	})

	workload.SetOwnerReferences(ownerReferences)

	return nil
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

func (webhook *DockyardsWorkload) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldWorkload, ok := oldObj.(*dockyardsv1.Workload)
	if !ok {
		return nil, apierrors.NewBadRequest("unexpected type")
	}

	newWorkload, ok := newObj.(*dockyardsv1.Workload)
	if !ok {
		return nil, apierrors.NewBadRequest("unexpected type")
	}

	if !webhook.validTemplateReferenceUpdate(oldWorkload, newWorkload) {
		forbidden := field.Forbidden(field.NewPath("spec", "workloadTemplateRef"), "reference is immutable")

		return nil, apierrors.NewInvalid(dockyardsv1.GroupVersion.WithKind(dockyardsv1.WorkloadKind).GroupKind(), oldWorkload.Name, field.ErrorList{forbidden})
	}

	if !webhook.validClusterReferenceUpdate(oldWorkload, newWorkload) {
		forbidden := field.Forbidden(field.NewPath("metadata", "labels", dockyardsv1.LabelClusterName), "reference is immutable")

		return nil, apierrors.NewInvalid(dockyardsv1.GroupVersion.WithKind(dockyardsv1.WorkloadKind).GroupKind(), oldWorkload.Name, field.ErrorList{forbidden})
	}

	if !webhook.validOrganizationReferenceUpdate(oldWorkload, newWorkload) {
		forbidden := field.Forbidden(field.NewPath("metadata", "labels", dockyardsv1.LabelOrganizationName), "reference is immutable")

		return nil, apierrors.NewInvalid(dockyardsv1.GroupVersion.WithKind(dockyardsv1.WorkloadKind).GroupKind(), oldWorkload.Name, field.ErrorList{forbidden})
	}

	return webhook.validate(ctx, newWorkload)
}

func (webhook *DockyardsWorkload) validate(ctx context.Context, workload *dockyardsv1.Workload) (admission.Warnings, error) {
	var allWarnings admission.Warnings
	var allErrors field.ErrorList

	if workload.Labels[dockyardsv1.LabelOrganizationName] == "" {
		required := field.Required(field.NewPath("metadata", "labels", dockyardsv1.LabelOrganizationName), "mandatory label")
		allErrors = append(allErrors, required)
	}

	if workload.Labels[dockyardsv1.LabelClusterName] == "" {
		required := field.Required(field.NewPath("metadata", "labels", dockyardsv1.LabelClusterName), "mandatory label")
		allErrors = append(allErrors, required)
	}

	if len(allErrors) == 0 {
		webhook.validateClusterReference(ctx, workload, &allErrors)
	}

	if workload.Spec.WorkloadTemplateInput != nil { //nolint:staticcheck
		allWarnings = append(allWarnings, "ignoring deprecated field workloadTemplateInput")
	}

	if workload.Spec.WorkloadTemplateRef == nil {
		if len(allErrors) > 0 {
			return allWarnings, apierrors.NewInvalid(dockyardsv1.GroupVersion.WithKind(dockyardsv1.WorkloadKind).GroupKind(), workload.Name, allErrors)
		}

		return allWarnings, nil
	}

	objectKey := client.ObjectKey{
		Name:      workload.Spec.WorkloadTemplateRef.Name,
		Namespace: workload.Namespace,
	}

	if workload.Spec.WorkloadTemplateRef.Namespace != nil {
		objectKey.Namespace = *workload.Spec.WorkloadTemplateRef.Namespace
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
		if len(allErrors) > 0 {
			return allWarnings, apierrors.NewInvalid(dockyardsv1.GroupVersion.WithKind(dockyardsv1.WorkloadKind).GroupKind(), workload.Name, allErrors)
		}

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

	if len(allErrors) > 0 {
		return allWarnings, apierrors.NewInvalid(dockyardsv1.GroupVersion.WithKind(dockyardsv1.WorkloadKind).GroupKind(), workload.Name, allErrors)
	}

	return allWarnings, nil
}

func (webhook *DockyardsWorkload) validClusterReferenceUpdate(oldWorkload, newWorkload *dockyardsv1.Workload) bool {
	return oldWorkload.Labels[dockyardsv1.LabelClusterName] == newWorkload.Labels[dockyardsv1.LabelClusterName]
}

func (webhook *DockyardsWorkload) validOrganizationReferenceUpdate(oldWorkload, newWorkload *dockyardsv1.Workload) bool {
	return oldWorkload.Labels[dockyardsv1.LabelOrganizationName] == newWorkload.Labels[dockyardsv1.LabelOrganizationName]
}

func (webhook *DockyardsWorkload) validTemplateReferenceUpdate(oldWorkload, newWorkload *dockyardsv1.Workload) bool {
	if oldWorkload.Spec.WorkloadTemplateRef == nil {
		return true
	}

	if oldWorkload.Spec.WorkloadTemplateRef != nil && newWorkload.Spec.WorkloadTemplateRef == nil {
		return false
	}

	if newWorkload.Spec.WorkloadTemplateRef.Name != oldWorkload.Spec.WorkloadTemplateRef.Name {
		return false
	}

	return true
}

func (webhook *DockyardsWorkload) validateClusterReference(ctx context.Context, workload *dockyardsv1.Workload, allErrors *field.ErrorList) {
	clusterName := workload.Labels[dockyardsv1.LabelClusterName]

	clusterOwnerReference, hasClusterOwnerReference := findClusterOwnerReference(workload)
	if hasClusterOwnerReference && clusterOwnerReference.Name != clusterName {
		invalid := field.Invalid(field.NewPath("metadata", "ownerReferences"), clusterOwnerReference.Name, "cluster owner reference must match cluster label")
		*allErrors = append(*allErrors, invalid)

		return
	}

	cluster, err := webhook.getCluster(ctx, workload.Namespace, clusterName)
	if apierrors.IsNotFound(err) {
		notFound := field.NotFound(field.NewPath("metadata", "labels", dockyardsv1.LabelClusterName), clusterName)
		*allErrors = append(*allErrors, notFound)

		return
	}

	if err != nil {
		internal := field.InternalError(field.NewPath("metadata", "labels", dockyardsv1.LabelClusterName), err)
		*allErrors = append(*allErrors, internal)

		return
	}

	if hasClusterOwnerReference && clusterOwnerReference.UID != cluster.UID {
		invalid := field.Invalid(field.NewPath("metadata", "ownerReferences"), clusterOwnerReference.UID, "cluster owner reference UID does not match cluster")
		*allErrors = append(*allErrors, invalid)

		return
	}

	clusterOrganizationName := cluster.Labels[dockyardsv1.LabelOrganizationName]
	workloadOrganizationName := workload.Labels[dockyardsv1.LabelOrganizationName]
	if clusterOrganizationName != "" && workloadOrganizationName != clusterOrganizationName {
		invalid := field.Invalid(field.NewPath("metadata", "labels", dockyardsv1.LabelOrganizationName), workloadOrganizationName, "organization label must match cluster owner organization")
		*allErrors = append(*allErrors, invalid)
	}
}

func (webhook *DockyardsWorkload) getCluster(ctx context.Context, namespace, name string) (*dockyardsv1.Cluster, error) {
	objectKey := client.ObjectKey{Namespace: namespace, Name: name}

	var cluster dockyardsv1.Cluster
	err := webhook.Client.Get(ctx, objectKey, &cluster)
	if err != nil {
		return nil, err
	}

	return &cluster, nil
}

func hasClusterOwnerReference(workload *dockyardsv1.Workload) bool {
	_, found := findClusterOwnerReference(workload)

	return found
}

func findClusterOwnerReference(workload *dockyardsv1.Workload) (metav1.OwnerReference, bool) {
	for _, ownerReference := range workload.OwnerReferences {
		if ownerReference.Kind != dockyardsv1.ClusterKind {
			continue
		}

		groupVersion, err := schema.ParseGroupVersion(ownerReference.APIVersion)
		if err != nil {
			continue
		}

		if groupVersion.Group != dockyardsv1.GroupVersion.Group {
			continue
		}

		return ownerReference, true
	}

	return metav1.OwnerReference{}, false
}
