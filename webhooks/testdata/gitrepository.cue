package template

import (
	dockyardsv1 "bitbucket.org/sudosweden/dockyards-backend/pkg/api/v1alpha3"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
)

#cluster:  dockyardsv1.#Cluster
#workload: dockyardsv1.#Workload

gitRepository: sourcev1.#GitRepository & {
	metadata: {
		name:      #cluster.metadata.name + "-" + #workload.metadata.name
		namespace: #workload.metadata.namespace
	}
}
