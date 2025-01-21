package template

import (
	"strings"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	dockyardsv1 "bitbucket.org/sudosweden/dockyards-backend/pkg/api/v1alpha3"
)

#Input: {
	chart!:      string
	repository!: string & =~"^(http?s|oci)://.*$"
	version!:    string
	values?: [string]: _
}

#cluster: dockyardsv1.#Cluster

#workload: dockyardsv1.#Workload
#workload: spec: input: #Input

helmRepository: sourcev1.#HelmRepository & {
	apiVersion: "source.toolkit.fluxcd.io/v1"
	kind:       sourcev1.#HelmRepositoryKind
	metadata: {
		name:      #workload.metadata.name
		namespace: #workload.metadata.namespace
	}
	spec: {
		interval: "5m"
		url:      #workload.spec.input.repository
		if strings.HasPrefix(#workload.spec.input.repository, "oci://") {
			type: "oci"
		}
	}
}

helmRelease: helmv2.#HelmRelease & {
	apiVersion: "helm.toolkit.fluxcd.io/v2"
	kind:       helmv2.#HelmReleaseKind
	metadata: {
		name:      #workload.metadata.name
		namespace: #workload.metadata.namespace
	}
	spec: {
		chart: spec: {
			chart: #workload.spec.input.chart
			sourceRef: {
				kind: helmRepository.kind
				name: helmRepository.metadata.name
			}
			version: #workload.spec.input.version
		}
		install: {
			createNamespace: true
			remediation: retries: -1
		}
		interval: "5m"
		kubeConfig: {
			secretRef: name: #cluster.metadata.name + "-kubeconfig"
		}
		storageNamespace: #workload.spec.targetNamespace
		targetNamespace:  #workload.spec.targetNamespace
		if #workload.spec.input.values != _|_ {
			values: #workload.spec.input.values
		}
	}
}
