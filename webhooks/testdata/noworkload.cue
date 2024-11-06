package template

import (
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	dockyardsv1 "bitbucket.org/sudosweden/dockyards-backend/pkg/api/v1alpha3"
)

#cluster: dockyardsv1.#Cluster

gitRepository: sourcev1.#GitRepository & {
	metadata: name: #cluster.metadata.name
}
