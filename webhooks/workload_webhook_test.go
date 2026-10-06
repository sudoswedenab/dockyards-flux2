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

package webhooks_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	dockyardsv1 "github.com/sudoswedenab/dockyards-backend/api/v1alpha3"
	"github.com/sudoswedenab/dockyards-flux2/webhooks"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDockyardsWorkload_ValidateCreate(t *testing.T) {
	tt := []struct {
		name              string
		workloadTemplate  dockyardsv1.WorkloadTemplate
		workload          dockyardsv1.Workload
		skipDefaultLabels bool
		expected          error
	}{
		{
			name: "test missing mandatory labels",
			workloadTemplate: dockyardsv1.WorkloadTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "dockyards-test",
				},
				Spec: dockyardsv1.WorkloadTemplateSpec{
					Source: mustReadAll("testdata/noinput.cue"),
				},
			},
			workload: dockyardsv1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-missing-mandatory-labels",
					Namespace: "testing",
				},
				Spec: dockyardsv1.WorkloadSpec{
					WorkloadTemplateRef: &corev1.TypedObjectReference{
						Kind:      dockyardsv1.WorkloadTemplateKind,
						Name:      "test",
						Namespace: ptr.To("dockyards-test"),
					},
				},
			},
			skipDefaultLabels: true,
			expected: apierrors.NewInvalid(
				dockyardsv1.GroupVersion.WithKind(dockyardsv1.WorkloadKind).GroupKind(),
				"test-missing-mandatory-labels",
				field.ErrorList{
					field.Required(field.NewPath("metadata", "labels", dockyardsv1.LabelOrganizationName), "mandatory label"),
					field.Required(field.NewPath("metadata", "labels", dockyardsv1.LabelClusterName), "mandatory label"),
				},
			),
		},
		{
			name: "test cluster owner reference must match cluster label",
			workloadTemplate: dockyardsv1.WorkloadTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "dockyards-test",
				},
				Spec: dockyardsv1.WorkloadTemplateSpec{
					Source: mustReadAll("testdata/noinput.cue"),
				},
			},
			workload: dockyardsv1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster-owner-reference-mismatch",
					Namespace: "testing",
					Labels: map[string]string{
						dockyardsv1.LabelOrganizationName: "test-org",
						dockyardsv1.LabelClusterName:      "test-cluster",
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: dockyardsv1.GroupVersion.String(),
							Kind:       dockyardsv1.ClusterKind,
							Name:       "other-cluster",
							UID:        "test-cluster-uid",
						},
					},
				},
				Spec: dockyardsv1.WorkloadSpec{
					WorkloadTemplateRef: &corev1.TypedObjectReference{
						Kind:      dockyardsv1.WorkloadTemplateKind,
						Name:      "test",
						Namespace: ptr.To("dockyards-test"),
					},
				},
			},
			expected: apierrors.NewInvalid(
				dockyardsv1.GroupVersion.WithKind(dockyardsv1.WorkloadKind).GroupKind(),
				"test-cluster-owner-reference-mismatch",
				field.ErrorList{
					field.Invalid(field.NewPath("metadata", "ownerReferences"), "other-cluster", "cluster owner reference must match cluster label"),
				},
			),
		},
		{
			name: "test template not found",
			workloadTemplate: dockyardsv1.WorkloadTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "dockyards-test",
				},
				Spec: dockyardsv1.WorkloadTemplateSpec{
					Source: mustReadAll("testdata/noinput.cue"),
				},
			},
			workload: dockyardsv1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-not-found",
					Namespace: "testing",
				},
				Spec: dockyardsv1.WorkloadSpec{
					WorkloadTemplateRef: &corev1.TypedObjectReference{
						Kind:      dockyardsv1.WorkloadTemplateKind,
						Name:      "test-not-found",
						Namespace: ptr.To("dockyards-test"),
					},
				},
			},
			expected: apierrors.NewInvalid(
				dockyardsv1.GroupVersion.WithKind(dockyardsv1.WorkloadKind).GroupKind(),
				"test-not-found",
				field.ErrorList{
					field.NotFound(field.NewPath("spec", "workloadTemplateRef"), "test-not-found"),
				},
			),
		},
		{
			name: "test input not supported on template",
			workloadTemplate: dockyardsv1.WorkloadTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "dockyards-test",
				},
				Spec: dockyardsv1.WorkloadTemplateSpec{
					Source: mustReadAll("testdata/noinput.cue"),
				},
			},
			workload: dockyardsv1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-input-not-supported",
					Namespace: "testing",
				},
				Spec: dockyardsv1.WorkloadSpec{
					WorkloadTemplateRef: &corev1.TypedObjectReference{
						Kind:      dockyardsv1.WorkloadTemplateKind,
						Name:      "test",
						Namespace: ptr.To("dockyards-test"),
					},
					Input: &apiextensionsv1.JSON{
						Raw: []byte(`{"test":true}`),
					},
				},
			},
			expected: apierrors.NewInvalid(
				dockyardsv1.GroupVersion.WithKind(dockyardsv1.WorkloadKind).GroupKind(),
				"test-input-not-supported",
				field.ErrorList{
					field.Forbidden(field.NewPath("spec", "input"), "input not supported on template"),
				},
			),
		},
		{
			name: "test defaults",
			workloadTemplate: dockyardsv1.WorkloadTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "dockyards-test",
				},
				Spec: dockyardsv1.WorkloadTemplateSpec{
					Source: mustReadAll("testdata/defaults.cue"),
				},
			},
			workload: dockyardsv1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-defaults",
					Namespace: "testing",
				},
				Spec: dockyardsv1.WorkloadSpec{
					WorkloadTemplateRef: &corev1.TypedObjectReference{
						Kind:      dockyardsv1.WorkloadTemplateKind,
						Name:      "test",
						Namespace: ptr.To("dockyards-test"),
					},
				},
			},
		},
		{
			name: "test input defaults",
			workloadTemplate: dockyardsv1.WorkloadTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "dockyards-test",
				},
				Spec: dockyardsv1.WorkloadTemplateSpec{
					Source: mustReadAll("testdata/defaults.cue"),
				},
			},
			workload: dockyardsv1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-input-defaults",
					Namespace: "testing",
				},
				Spec: dockyardsv1.WorkloadSpec{
					WorkloadTemplateRef: &corev1.TypedObjectReference{
						Kind:      dockyardsv1.WorkloadTemplateKind,
						Name:      "test",
						Namespace: ptr.To("dockyards-test"),
					},
					Input: &apiextensionsv1.JSON{
						Raw: []byte(`{"test":true,"count":1}`),
					},
				},
			},
		},
		{
			name: "test not allowed defaults",
			workloadTemplate: dockyardsv1.WorkloadTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "dockyards-test",
				},
				Spec: dockyardsv1.WorkloadTemplateSpec{
					Source: mustReadAll("testdata/defaults.cue"),
				},
			},
			workload: dockyardsv1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-not-allowed-defaults",
					Namespace: "testing",
				},
				Spec: dockyardsv1.WorkloadSpec{
					WorkloadTemplateRef: &corev1.TypedObjectReference{
						Kind:      dockyardsv1.WorkloadTemplateKind,
						Name:      "test",
						Namespace: ptr.To("dockyards-test"),
					},
					Input: &apiextensionsv1.JSON{
						Raw: []byte(`{"qwfp":true}`),
					},
				},
			},
			expected: apierrors.NewInvalid(
				dockyardsv1.GroupVersion.WithKind(dockyardsv1.WorkloadKind).GroupKind(),
				"test-not-allowed-defaults",
				field.ErrorList{
					field.Forbidden(field.NewPath("spec", "input"), "#Input.qwfp: field not allowed"),
				},
			),
		},
		{
			name: "test required field",
			workloadTemplate: dockyardsv1.WorkloadTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "dockyards-test",
				},
				Spec: dockyardsv1.WorkloadTemplateSpec{
					Source: mustReadAll("testdata/required.cue"),
				},
			},
			workload: dockyardsv1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-required",
					Namespace: "testing",
				},
				Spec: dockyardsv1.WorkloadSpec{
					WorkloadTemplateRef: &corev1.TypedObjectReference{
						Kind:      dockyardsv1.WorkloadTemplateKind,
						Name:      "test",
						Namespace: ptr.To("dockyards-test"),
					},
					Input: &apiextensionsv1.JSON{
						Raw: []byte(`{"test":"qwfp"}`),
					},
				},
			},
		},
		{
			name: "test required field not present",
			workloadTemplate: dockyardsv1.WorkloadTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "dockyards-test",
				},
				Spec: dockyardsv1.WorkloadTemplateSpec{
					Source: mustReadAll("testdata/required.cue"),
				},
			},
			workload: dockyardsv1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-required-not-present",
					Namespace: "testing",
				},
				Spec: dockyardsv1.WorkloadSpec{
					WorkloadTemplateRef: &corev1.TypedObjectReference{
						Kind:      dockyardsv1.WorkloadTemplateKind,
						Name:      "test",
						Namespace: ptr.To("dockyards-test"),
					},
					Input: &apiextensionsv1.JSON{
						Raw: []byte(`{}`),
					},
				},
			},
			expected: apierrors.NewInvalid(
				dockyardsv1.GroupVersion.WithKind(dockyardsv1.WorkloadKind).GroupKind(),
				"test-required-not-present",
				field.ErrorList{
					field.Forbidden(field.NewPath("spec", "input"), "#Input.test: field is required but not present"),
				},
			),
		},
		{
			name: "test conflicting type",
			workloadTemplate: dockyardsv1.WorkloadTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "dockyards-test",
				},
				Spec: dockyardsv1.WorkloadTemplateSpec{
					Source: mustReadAll("testdata/required.cue"),
				},
			},
			workload: dockyardsv1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-conflicting-type",
					Namespace: "testing",
				},
				Spec: dockyardsv1.WorkloadSpec{
					WorkloadTemplateRef: &corev1.TypedObjectReference{
						Kind:      dockyardsv1.WorkloadTemplateKind,
						Name:      "test",
						Namespace: ptr.To("dockyards-test"),
					},
					Input: &apiextensionsv1.JSON{
						Raw: []byte(`{"test":true}`),
					},
				},
			},
			expected: apierrors.NewInvalid(
				dockyardsv1.GroupVersion.WithKind(dockyardsv1.WorkloadKind).GroupKind(),
				"test-conflicting-type",
				field.ErrorList{
					field.Forbidden(field.NewPath("spec", "input"), "#Input.test: conflicting values string and true (mismatched types string and bool)"),
				},
			),
		},
		{
			name: "test only one of",
			workloadTemplate: dockyardsv1.WorkloadTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "dockyards-test",
				},
				Spec: dockyardsv1.WorkloadTemplateSpec{
					Source: mustReadAll("testdata/oneof.cue"),
				},
			},
			workload: dockyardsv1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-only-one-of",
					Namespace: "testing",
				},
				Spec: dockyardsv1.WorkloadSpec{
					WorkloadTemplateRef: &corev1.TypedObjectReference{
						Kind:      dockyardsv1.WorkloadTemplateKind,
						Name:      "test",
						Namespace: ptr.To("dockyards-test"),
					},
					Input: &apiextensionsv1.JSON{
						Raw: []byte(`{"a":true}`),
					},
				},
			},
		},
		{
			name: "test both one of",
			workloadTemplate: dockyardsv1.WorkloadTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "dockyards-test",
				},
				Spec: dockyardsv1.WorkloadTemplateSpec{
					Source: mustReadAll("testdata/oneof.cue"),
				},
			},
			workload: dockyardsv1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-both-one-of",
					Namespace: "testing",
				},
				Spec: dockyardsv1.WorkloadSpec{
					WorkloadTemplateRef: &corev1.TypedObjectReference{
						Kind:      dockyardsv1.WorkloadTemplateKind,
						Name:      "test",
						Namespace: ptr.To("dockyards-test"),
					},
					Input: &apiextensionsv1.JSON{
						Raw: []byte(`{"a":true,"b":true}`),
					},
				},
			},
			expected: apierrors.NewInvalid(
				dockyardsv1.GroupVersion.WithKind(dockyardsv1.WorkloadKind).GroupKind(),
				"test-both-one-of",
				field.ErrorList{
					field.Forbidden(field.NewPath("spec", "input"), "#Input: 2 errors in empty disjunction:"),
					field.Forbidden(field.NewPath("spec", "input"), "#Input.a: field not allowed"),
					field.Forbidden(field.NewPath("spec", "input"), "#Input.b: field not allowed"),
				},
			),
		},
		{
			name: "test regexp",
			workloadTemplate: dockyardsv1.WorkloadTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "dockyards-test",
				},
				Spec: dockyardsv1.WorkloadTemplateSpec{
					Source: mustReadAll("testdata/regexp.cue"),
				},
			},
			workload: dockyardsv1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-regexp",
					Namespace: "testing",
				},
				Spec: dockyardsv1.WorkloadSpec{
					WorkloadTemplateRef: &corev1.TypedObjectReference{
						Kind:      dockyardsv1.WorkloadTemplateKind,
						Name:      "test",
						Namespace: ptr.To("dockyards-test"),
					},
					Input: &apiextensionsv1.JSON{
						Raw: []byte(`{"test":"abc"}`),
					},
				},
			},
		},
		{
			name: "test not matching regexp",
			workloadTemplate: dockyardsv1.WorkloadTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "dockyards-test",
				},
				Spec: dockyardsv1.WorkloadTemplateSpec{
					Source: mustReadAll("testdata/regexp.cue"),
				},
			},
			workload: dockyardsv1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-not-matching-regexp",
					Namespace: "testing",
				},
				Spec: dockyardsv1.WorkloadSpec{
					WorkloadTemplateRef: &corev1.TypedObjectReference{
						Kind:      dockyardsv1.WorkloadTemplateKind,
						Name:      "test",
						Namespace: ptr.To("dockyards-test"),
					},
					Input: &apiextensionsv1.JSON{
						Raw: []byte(`{"test":"ABC"}`),
					},
				},
			},
			expected: apierrors.NewInvalid(
				dockyardsv1.GroupVersion.WithKind(dockyardsv1.WorkloadKind).GroupKind(),
				"test-not-matching-regexp",
				field.ErrorList{
					field.Forbidden(field.NewPath("spec", "input"), `#Input.test: invalid value "ABC" (out of bound =~"^[a-z].*$")`),
				},
			),
		},
		{
			name: "test empty namespace",
			workloadTemplate: dockyardsv1.WorkloadTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "testing",
				},
				Spec: dockyardsv1.WorkloadTemplateSpec{
					Source: mustReadAll("testdata/noinput.cue"),
				},
			},
			workload: dockyardsv1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-empty-namespace",
					Namespace: "testing",
				},
				Spec: dockyardsv1.WorkloadSpec{
					WorkloadTemplateRef: &corev1.TypedObjectReference{
						Kind: dockyardsv1.WorkloadTemplateKind,
						Name: "test",
					},
				},
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.skipDefaultLabels {
				setDefaultWorkloadLabels(&tc.workload)
			}

			cluster := testCluster(tc.workload.Namespace, tc.workload.Labels[dockyardsv1.LabelClusterName], tc.workload.Labels[dockyardsv1.LabelOrganizationName])

			scheme := runtime.NewScheme()

			_ = dockyardsv1.AddToScheme(scheme)

			c := fake.
				NewClientBuilder().
				WithScheme(scheme).
				WithObjects(&tc.workloadTemplate, cluster).
				Build()

			webhook := webhooks.DockyardsWorkload{
				Client: c,
			}

			_, actual := webhook.ValidateCreate(context.Background(), &tc.workload)
			if !cmp.Equal(actual, tc.expected) {
				t.Errorf("diff: %s", cmp.Diff(tc.expected, actual))
			}
		})
	}
}

func TestDockyardsWorkload_ValidateUpdate(t *testing.T) {
	tt := []struct {
		name              string
		workloadTemplate  dockyardsv1.WorkloadTemplate
		oldWorkload       dockyardsv1.Workload
		newWorkload       dockyardsv1.Workload
		skipDefaultLabels bool
		expected          error
	}{
		{
			name: "test immutable cluster label",
			workloadTemplate: dockyardsv1.WorkloadTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "dockyards-test",
				},
				Spec: dockyardsv1.WorkloadTemplateSpec{
					Source: mustReadAll("testdata/defaults.cue"),
				},
			},
			oldWorkload: dockyardsv1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-immutable-cluster-label",
					Namespace: "testing",
					Labels: map[string]string{
						dockyardsv1.LabelOrganizationName: "test-org",
						dockyardsv1.LabelClusterName:      "cluster-a",
					},
				},
				Spec: dockyardsv1.WorkloadSpec{
					WorkloadTemplateRef: &corev1.TypedObjectReference{
						Kind:      dockyardsv1.WorkloadTemplateKind,
						Name:      "test",
						Namespace: ptr.To("dockyards-test"),
					},
				},
			},
			newWorkload: dockyardsv1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-immutable-cluster-label",
					Namespace: "testing",
					Labels: map[string]string{
						dockyardsv1.LabelOrganizationName: "test-org",
						dockyardsv1.LabelClusterName:      "cluster-b",
					},
				},
				Spec: dockyardsv1.WorkloadSpec{
					WorkloadTemplateRef: &corev1.TypedObjectReference{
						Kind:      dockyardsv1.WorkloadTemplateKind,
						Name:      "test",
						Namespace: ptr.To("dockyards-test"),
					},
				},
			},
			expected: apierrors.NewInvalid(
				dockyardsv1.GroupVersion.WithKind(dockyardsv1.WorkloadKind).GroupKind(),
				"test-immutable-cluster-label",
				field.ErrorList{
					field.Forbidden(field.NewPath("metadata", "labels", dockyardsv1.LabelClusterName), `reference is immutable`),
				},
			),
		},
		{
			name: "test immutable organization label",
			workloadTemplate: dockyardsv1.WorkloadTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "dockyards-test",
				},
				Spec: dockyardsv1.WorkloadTemplateSpec{
					Source: mustReadAll("testdata/defaults.cue"),
				},
			},
			oldWorkload: dockyardsv1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-immutable-organization-label",
					Namespace: "testing",
					Labels: map[string]string{
						dockyardsv1.LabelOrganizationName: "org-a",
						dockyardsv1.LabelClusterName:      "cluster-a",
					},
				},
				Spec: dockyardsv1.WorkloadSpec{
					WorkloadTemplateRef: &corev1.TypedObjectReference{
						Kind:      dockyardsv1.WorkloadTemplateKind,
						Name:      "test",
						Namespace: ptr.To("dockyards-test"),
					},
				},
			},
			newWorkload: dockyardsv1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-immutable-organization-label",
					Namespace: "testing",
					Labels: map[string]string{
						dockyardsv1.LabelOrganizationName: "org-b",
						dockyardsv1.LabelClusterName:      "cluster-a",
					},
				},
				Spec: dockyardsv1.WorkloadSpec{
					WorkloadTemplateRef: &corev1.TypedObjectReference{
						Kind:      dockyardsv1.WorkloadTemplateKind,
						Name:      "test",
						Namespace: ptr.To("dockyards-test"),
					},
				},
			},
			expected: apierrors.NewInvalid(
				dockyardsv1.GroupVersion.WithKind(dockyardsv1.WorkloadKind).GroupKind(),
				"test-immutable-organization-label",
				field.ErrorList{
					field.Forbidden(field.NewPath("metadata", "labels", dockyardsv1.LabelOrganizationName), `reference is immutable`),
				},
			),
		},
		{
			name: "test update empty reference",
			workloadTemplate: dockyardsv1.WorkloadTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "dockyards-test",
				},
				Spec: dockyardsv1.WorkloadTemplateSpec{
					Source: mustReadAll("testdata/defaults.cue"),
				},
			},
			oldWorkload: dockyardsv1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-empty-reference",
					Namespace: "testing",
				},
				Spec: dockyardsv1.WorkloadSpec{},
			},
			newWorkload: dockyardsv1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-empty-reference",
					Namespace: "testing",
				},
				Spec: dockyardsv1.WorkloadSpec{
					WorkloadTemplateRef: &corev1.TypedObjectReference{
						Kind:      dockyardsv1.WorkloadTemplateKind,
						Name:      "test",
						Namespace: ptr.To("dockyards-test"),
					},
				},
			},
		},
		{
			name: "test reference name",
			workloadTemplate: dockyardsv1.WorkloadTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "dockyards-test",
				},
				Spec: dockyardsv1.WorkloadTemplateSpec{
					Source: mustReadAll("testdata/defaults.cue"),
				},
			},
			oldWorkload: dockyardsv1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-reference-name",
					Namespace: "testing",
				},
				Spec: dockyardsv1.WorkloadSpec{
					WorkloadTemplateRef: &corev1.TypedObjectReference{
						Kind:      dockyardsv1.WorkloadTemplateKind,
						Name:      "test",
						Namespace: ptr.To("dockyards-test"),
					},
				},
			},
			newWorkload: dockyardsv1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-reference-name",
					Namespace: "testing",
				},
				Spec: dockyardsv1.WorkloadSpec{
					WorkloadTemplateRef: &corev1.TypedObjectReference{
						Kind:      dockyardsv1.WorkloadTemplateKind,
						Name:      "test-update",
						Namespace: ptr.To("dockyards-test"),
					},
				},
			},
			expected: apierrors.NewInvalid(
				dockyardsv1.GroupVersion.WithKind(dockyardsv1.WorkloadKind).GroupKind(),
				"test-reference-name",
				field.ErrorList{
					field.Forbidden(field.NewPath("spec", "workloadTemplateRef"), `reference is immutable`),
				},
			),
		},
		{
			name: "test remove reference",
			workloadTemplate: dockyardsv1.WorkloadTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "dockyards-test",
				},
				Spec: dockyardsv1.WorkloadTemplateSpec{
					Source: mustReadAll("testdata/defaults.cue"),
				},
			},
			oldWorkload: dockyardsv1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-remove-reference",
					Namespace: "testing",
				},
				Spec: dockyardsv1.WorkloadSpec{
					WorkloadTemplateRef: &corev1.TypedObjectReference{
						Kind:      dockyardsv1.WorkloadTemplateKind,
						Name:      "test",
						Namespace: ptr.To("dockyards-test"),
					},
				},
			},
			newWorkload: dockyardsv1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-remove-reference",
					Namespace: "testing",
				},
				Spec: dockyardsv1.WorkloadSpec{},
			},
			expected: apierrors.NewInvalid(
				dockyardsv1.GroupVersion.WithKind(dockyardsv1.WorkloadKind).GroupKind(),
				"test-remove-reference",
				field.ErrorList{
					field.Forbidden(field.NewPath("spec", "workloadTemplateRef"), `reference is immutable`),
				},
			),
		},
		{
			name: "test input",
			workloadTemplate: dockyardsv1.WorkloadTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "dockyards-test",
				},
				Spec: dockyardsv1.WorkloadTemplateSpec{
					Source: mustReadAll("testdata/defaults.cue"),
				},
			},
			oldWorkload: dockyardsv1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-remove-reference",
					Namespace: "testing",
				},
				Spec: dockyardsv1.WorkloadSpec{
					WorkloadTemplateRef: &corev1.TypedObjectReference{
						Kind:      dockyardsv1.WorkloadTemplateKind,
						Name:      "test",
						Namespace: ptr.To("dockyards-test"),
					},
					Input: &apiextensionsv1.JSON{
						Raw: []byte(`{"count":1}`),
					},
				},
			},
			newWorkload: dockyardsv1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-remove-reference",
					Namespace: "testing",
				},
				Spec: dockyardsv1.WorkloadSpec{
					WorkloadTemplateRef: &corev1.TypedObjectReference{
						Kind:      dockyardsv1.WorkloadTemplateKind,
						Name:      "test",
						Namespace: ptr.To("dockyards-test"),
					},
					Input: &apiextensionsv1.JSON{
						Raw: []byte(`{"count":2}`),
					},
				},
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.skipDefaultLabels {
				setDefaultWorkloadLabels(&tc.oldWorkload)
				setDefaultWorkloadLabels(&tc.newWorkload)
			}

			cluster := testCluster(tc.newWorkload.Namespace, tc.newWorkload.Labels[dockyardsv1.LabelClusterName], tc.newWorkload.Labels[dockyardsv1.LabelOrganizationName])

			scheme := runtime.NewScheme()

			_ = dockyardsv1.AddToScheme(scheme)

			c := fake.
				NewClientBuilder().
				WithScheme(scheme).
				WithObjects(&tc.workloadTemplate, cluster).
				Build()

			webhook := webhooks.DockyardsWorkload{
				Client: c,
			}

			_, actual := webhook.ValidateUpdate(context.Background(), &tc.oldWorkload, &tc.newWorkload)
			if !cmp.Equal(actual, tc.expected) {
				t.Errorf("diff: %s", cmp.Diff(tc.expected, actual))
			}
		})
	}
}

func TestDockyardsWorkload_Default(t *testing.T) {
	scheme := runtime.NewScheme()

	_ = dockyardsv1.AddToScheme(scheme)

	cluster := testCluster("testing", "test-cluster", "test-org")

	c := fake.
		NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		Build()

	webhook := webhooks.DockyardsWorkload{
		Client: c,
	}

	t.Run("adds owner reference from cluster label", func(t *testing.T) {
		workload := dockyardsv1.Workload{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-workload",
				Namespace: "testing",
				Labels: map[string]string{
					dockyardsv1.LabelOrganizationName: "test-org",
					dockyardsv1.LabelClusterName:      "test-cluster",
				},
			},
		}

		err := webhook.Default(context.Background(), &workload)
		if err != nil {
			t.Fatal(err)
		}

		if len(workload.OwnerReferences) != 1 {
			t.Fatalf("expected 1 owner reference, got %d", len(workload.OwnerReferences))
		}

		expectedOwnerReference := metav1.OwnerReference{
			APIVersion: dockyardsv1.GroupVersion.String(),
			Kind:       dockyardsv1.ClusterKind,
			Name:       cluster.Name,
			UID:        cluster.UID,
		}

		if !cmp.Equal(workload.OwnerReferences[0], expectedOwnerReference) {
			t.Errorf("diff: %s", cmp.Diff(expectedOwnerReference, workload.OwnerReferences[0]))
		}
	})

	t.Run("does not override existing owner reference", func(t *testing.T) {
		workload := dockyardsv1.Workload{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-workload-existing-owner",
				Namespace: "testing",
				Labels: map[string]string{
					dockyardsv1.LabelOrganizationName: "test-org",
					dockyardsv1.LabelClusterName:      "test-cluster",
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: dockyardsv1.GroupVersion.String(),
						Kind:       dockyardsv1.ClusterKind,
						Name:       "test-cluster",
						UID:        cluster.UID,
					},
				},
			},
		}

		err := webhook.Default(context.Background(), &workload)
		if err != nil {
			t.Fatal(err)
		}

		if len(workload.OwnerReferences) != 1 {
			t.Fatalf("expected 1 owner reference, got %d", len(workload.OwnerReferences))
		}
	})
}

func setDefaultWorkloadLabels(workload *dockyardsv1.Workload) {
	if workload.ObjectMeta.Labels == nil {
		workload.ObjectMeta.Labels = map[string]string{}
	}

	if workload.ObjectMeta.Labels[dockyardsv1.LabelOrganizationName] == "" {
		workload.ObjectMeta.Labels[dockyardsv1.LabelOrganizationName] = "test-org"
	}

	if workload.ObjectMeta.Labels[dockyardsv1.LabelClusterName] == "" {
		workload.ObjectMeta.Labels[dockyardsv1.LabelClusterName] = "test-cluster"
	}
}

func testCluster(namespace, clusterName, organizationName string) *dockyardsv1.Cluster {
	if namespace == "" {
		namespace = "testing"
	}

	if clusterName == "" {
		clusterName = "test-cluster"
	}

	labels := map[string]string{}
	if organizationName != "" {
		labels[dockyardsv1.LabelOrganizationName] = organizationName
	}

	return &dockyardsv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName,
			Namespace: namespace,
			Labels:    labels,
			UID:       "test-cluster-uid",
		},
	}
}
