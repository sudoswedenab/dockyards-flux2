package mockcrds

import (
	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	dockyardsv1 "github.com/sudoswedenab/dockyards-backend/api/v1alpha3"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

var (
	DockyardsCluster          = mockCRD(dockyardsv1.ClusterKind, "clusters", dockyardsv1.GroupVersion.Group, dockyardsv1.GroupVersion.Version)
	DockyardsWorkload         = mockCRD(dockyardsv1.WorkloadKind, "workloads", dockyardsv1.GroupVersion.Group, dockyardsv1.GroupVersion.Version)
	DockyardsWorkloadTemplate = mockCRD(dockyardsv1.WorkloadTemplateKind, "workloadtemplates", dockyardsv1.GroupVersion.Group, dockyardsv1.GroupVersion.Version)
	HelmHelmRelease           = mockCRD(helmv2.HelmReleaseKind, "helmreleases", helmv2.GroupVersion.Group, helmv2.GroupVersion.Version)
	KustomizeKustomization    = mockCRD(kustomizev1.KustomizationKind, "kustomizations", kustomizev1.GroupVersion.Group, kustomizev1.GroupVersion.Version)
	SourceGitRepository       = mockCRD(sourcev1.GitRepositoryKind, "gitrepositories", sourcev1.GroupVersion.Group, sourcev1.GroupVersion.Version)
	DockyardsWorktree         = mockCRD(dockyardsv1.WorktreeKind, "worktrees", dockyardsv1.GroupVersion.Group, dockyardsv1.GroupVersion.Version)

	CRDs = []*apiextensionsv1.CustomResourceDefinition{
		DockyardsCluster,
		DockyardsWorkload,
		DockyardsWorkloadTemplate,
		DockyardsWorktree,
		HelmHelmRelease,
		KustomizeKustomization,
		SourceGitRepository,
	}
)

func mockCRD(kind, plural, group, version string) *apiextensionsv1.CustomResourceDefinition {
	return &apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: apiextensionsv1.SchemeGroupVersion.String(),
			Kind:       "CustomResourceDefinition",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: plural + "." + group,
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: group,
			Scope: apiextensionsv1.NamespaceScoped,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural: plural,
				Kind:   kind,
			},
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    version,
					Served:  true,
					Storage: true,
					Subresources: &apiextensionsv1.CustomResourceSubresources{
						Status: &apiextensionsv1.CustomResourceSubresourceStatus{},
					},
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							Type: "object",
							Properties: map[string]apiextensionsv1.JSONSchemaProps{
								"spec": {
									Type:                   "object",
									XPreserveUnknownFields: ptr.To(true),
								},
								"status": {
									Type:                   "object",
									XPreserveUnknownFields: ptr.To(true),
								},
							},
						},
					},
				},
			},
		},
	}
}
