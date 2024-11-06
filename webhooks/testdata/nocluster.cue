package template

import (
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	dockyardsv1 "bitbucket.org/sudosweden/dockyards-backend/pkg/api/v1alpha3"
)

#workload: dockyardsv1.#Workload

gitRepository: sourcev1.#GitRepository & {
	metadata: name: #workload.metadata.name
}
