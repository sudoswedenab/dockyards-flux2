package webhooks_test

import (
	"context"
	"testing"

	"bitbucket.org/sudosweden/dockyards-flux2/webhooks"
	"github.com/google/go-cmp/cmp"
	dockyardsv1 "github.com/sudoswedenab/dockyards-backend/api/v1alpha3"
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
		name             string
		workloadTemplate dockyardsv1.WorkloadTemplate
		workload         dockyardsv1.Workload
		expected         error
	}{
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
			scheme := runtime.NewScheme()

			_ = dockyardsv1.AddToScheme(scheme)

			c := fake.
				NewClientBuilder().
				WithScheme(scheme).
				WithObjects(&tc.workloadTemplate).
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
		name             string
		workloadTemplate dockyardsv1.WorkloadTemplate
		oldWorkload      dockyardsv1.Workload
		newWorkload      dockyardsv1.Workload
		expected         error
	}{
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
			scheme := runtime.NewScheme()

			_ = dockyardsv1.AddToScheme(scheme)

			c := fake.
				NewClientBuilder().
				WithScheme(scheme).
				WithObjects(&tc.workloadTemplate).
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
