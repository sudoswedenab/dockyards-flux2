package webhooks_test

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	dockyardsv1 "github.com/sudoswedenab/dockyards-backend/api/v1alpha3"
	"github.com/sudoswedenab/dockyards-flux2/webhooks"
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
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

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
						"build constraints exclude all CUE files in .:\n    webhooks/template.cue: no package name",
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
		{
			name: "test unknown import",
			workloadTemplate: dockyardsv1.WorkloadTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-unknown-import",
					Namespace: "testing",
				},
				Spec: dockyardsv1.WorkloadTemplateSpec{
					Source: mustReadAll("testdata/unknownimport.cue"),
					Type:   dockyardsv1.WorkloadTemplateTypeCue,
				},
			},
			expected: apierrors.NewInvalid(
				dockyardsv1.GroupVersion.WithKind(dockyardsv1.WorkloadTemplateKind).GroupKind(),
				"test-unknown-import",
				field.ErrorList{
					field.Invalid(
						field.NewPath("spec", "source"),
						`"dockyards.io/unknown"`,
						`import failed: `+wd+`/template.cue:4:2: `+
							`cannot find package "dockyards.io/unknown": cannot find module providing `+
							`package dockyards.io/unknown`,
					),
				},
			),
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
