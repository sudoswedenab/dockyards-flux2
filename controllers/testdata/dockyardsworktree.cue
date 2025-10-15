package template

import (
	dockyardsv1 "github.com/sudoswedenab/dockyards-backend/api/v1alpha3"
)

#cluster:  dockyardsv1.#Cluster
#workload: dockyardsv1.#Workload

worktree: dockyardsv1.#Worktree & {
	apiVersion: "dockyards.io/v1alpha3"
	kind:       dockyardsv1.#WorktreeKind
	metadata: {
		name:      #workload.metadata.name
		namespace: #workload.metadata.namespace
	}
	spec: {
		files: {
			"test": 'hello'
		}
	}
}
