package dockyardsutil

import (
	"context"

	"bitbucket.org/sudosweden/dockyards-backend/pkg/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetOwnerHelmDeployment(ctx context.Context, r client.Client, o client.Object) (*v1alpha1.HelmDeployment, error) {
	for _, ownerReference := range o.GetOwnerReferences() {
		if ownerReference.Kind != v1alpha1.HelmDeploymentKind {
			continue
		}

		groupVersion, err := schema.ParseGroupVersion(ownerReference.APIVersion)
		if err != nil {
			return nil, err
		}

		if groupVersion.Group == v1alpha1.GroupVersion.Group {
			objectKey := client.ObjectKey{
				Name:      ownerReference.Name,
				Namespace: o.GetNamespace(),
			}

			var helmDeployment v1alpha1.HelmDeployment
			err := r.Get(ctx, objectKey, &helmDeployment)
			if err != nil {
				return nil, err
			}

			return &helmDeployment, nil
		}

	}
	return nil, nil
}

func GetOwnerDeployment(ctx context.Context, r client.Client, o client.Object) (*v1alpha1.Deployment, error) {
	for _, ownerReference := range o.GetOwnerReferences() {
		if ownerReference.Kind != v1alpha1.DeploymentKind {
			continue
		}

		groupVersion, err := schema.ParseGroupVersion(ownerReference.APIVersion)
		if err != nil {
			return nil, err
		}

		if groupVersion.Group == v1alpha1.GroupVersion.Group {
			objectKey := client.ObjectKey{
				Name:      ownerReference.Name,
				Namespace: o.GetNamespace(),
			}

			var helmDeployment v1alpha1.Deployment
			err := r.Get(ctx, objectKey, &helmDeployment)
			if err != nil {
				return nil, err
			}

			return &helmDeployment, nil
		}

	}
	return nil, nil
}
