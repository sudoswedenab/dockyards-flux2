package template

import (
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	dockyardsv1 "bitbucket.org/sudosweden/dockyards-backend/pkg/api/v1alpha3"
)

#cluster:  dockyardsv1.#Cluster
#workload: dockyardsv1.#Workload

gitRepository: sourcev1.#GitRepository & {
	apiVersion: "source.toolkit.fluxcd.io/v1"
	kind:       sourcev1.#GitRepositoryKind
	metadata: {
		name:      #cluster.metadata.name + "-" + #workload.metadata.name
		namespace: #workload.metadata.namespace
	}
	spec: {
		interval: "5m"
		url:      "https://github.com/stefanprodan/podinfo"
		ref: {
			branch: "main"
		}
	}
}
