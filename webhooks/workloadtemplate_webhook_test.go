package webhooks_test

import (
	"context"
	"io"
	"os"
	"testing"

	dockyardsv1 "bitbucket.org/sudosweden/dockyards-backend/pkg/api/v1alpha3"
	"bitbucket.org/sudosweden/dockyards-flux2/webhooks"
	"github.com/google/go-cmp/cmp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func mustReadAll(filename string) string {
	file, err := os.Open(filename)
	if err != nil {
		panic(err)
	}

	b, err := io.ReadAll(file)
	if err != nil {
		panic(err)
	}

	return string(b)
}

func TestWorkloadTemplateWebhook_Create(t *testing.T) {
	tt := []struct {
		name             string
		workloadTemplate dockyardsv1.WorkloadTemplate
		expected         error
	}{
		{
			name: "test empty source",
			workloadTemplate: dockyardsv1.WorkloadTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-empty",
					Namespace: "testing",
				},
				Spec: dockyardsv1.WorkloadTemplateSpec{
					Source: "",
					Type:   dockyardsv1.WorkloadTemplateTypeCue,
				},
			},
			expected: apierrors.NewInvalid(
				dockyardsv1.GroupVersion.WithKind(dockyardsv1.WorkloadTemplateKind).GroupKind(),
				"test-empty",
				field.ErrorList{
					field.Invalid(
						field.NewPath("spec", "source"),
						"",
						"err: build constraints exclude all CUE files in .:\n    webhooks/template.cue: no package name",
					),
				},
			),
		},
		{
			name: "test no workload definition",
			workloadTemplate: dockyardsv1.WorkloadTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload-definition",
					Namespace: "testing",
				},
				Spec: dockyardsv1.WorkloadTemplateSpec{
					Source: mustReadAll("testdata/noworkload.cue"),
					Type:   dockyardsv1.WorkloadTemplateTypeCue,
				},
			},
			expected: apierrors.NewInvalid(
				dockyardsv1.GroupVersion.WithKind(dockyardsv1.WorkloadTemplateKind).GroupKind(),
				"test-workload-definition",
				field.ErrorList{
					field.Required(
						field.NewPath("spec", "source", "#workload"),
						"must have workload definition",
					),
				},
			),
		},
		{
			name: "test no cluster definition",
			workloadTemplate: dockyardsv1.WorkloadTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster-definition",
					Namespace: "testing",
				},
				Spec: dockyardsv1.WorkloadTemplateSpec{
					Source: mustReadAll("testdata/nocluster.cue"),
					Type:   dockyardsv1.WorkloadTemplateTypeCue,
				},
			},
			expected: apierrors.NewInvalid(
				dockyardsv1.GroupVersion.WithKind(dockyardsv1.WorkloadTemplateKind).GroupKind(),
				"test-cluster-definition",
				field.ErrorList{
					field.Required(
						field.NewPath("spec", "source", "#cluster"),
						"must have cluster definition",
					),
				},
			),
		},
		{
			name: "test git repository",
			workloadTemplate: dockyardsv1.WorkloadTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-git-repository",
					Namespace: "testing",
				},
				Spec: dockyardsv1.WorkloadTemplateSpec{
					Source: mustReadAll("testdata/gitrepository.cue"),
					Type:   dockyardsv1.WorkloadTemplateTypeCue,
				},
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			webhook := webhooks.DockyardsWorkloadTemplate{}

			_, actual := webhook.ValidateCreate(context.Background(), &tc.workloadTemplate)
			if !cmp.Equal(actual, tc.expected) {
				t.Errorf("diff: %s", cmp.Diff(tc.expected, actual))
			}
		})
	}
}
