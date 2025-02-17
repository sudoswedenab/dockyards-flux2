package template

import (
	corev1 "k8s.io/api/core/v1"
	dockyardsv1 "bitbucket.org/sudosweden/dockyards-backend/pkg/api/v1alpha3"
)

#cluster:  dockyardsv1.#Cluster
#workload: dockyardsv1.#Workload

secret: corev1.#Secret & {
	apiVersion: "v1"
	kind:       "Secret"
	metadata: {
		name:      #workload.metadata.name
		namespace: #workload.metadata.namespace
	}
	data: {
		"test": 'hello'
	}
}
