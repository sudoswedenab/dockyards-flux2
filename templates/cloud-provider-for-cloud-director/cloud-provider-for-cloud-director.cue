package template

import (
	"encoding/yaml"
	"encoding/base64"

	corev1 "k8s.io/api/core/v1"
	dockyardsv1 "bitbucket.org/sudosweden/dockyards-backend/pkg/api/v1alpha3"
	kustomize "sigs.k8s.io/kustomize/api/types"
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
)

#Input: {
	host!:                        =~"^https://.*$"
	org!:                         string
	vdc!:                         string
	enableVirtualServiceSharedIP: bool | *true
	network!:                     string
	vAppName!:                    string
	clusterID!:                   string
	apiToken!:                    string
}

#cluster: dockyardsv1.#Cluster

#workload: dockyardsv1.#Workload
#workload: spec: input: #Input

_config: {
	vcd: {
		host: #workload.spec.input.host
		org:  #workload.spec.input.org
		vdc:  #workload.spec.input.vdc
	}
	loadbalancer: {
		enableVirtualServiceSharedIP: #workload.spec.input.enableVirtualServiceSharedIP
		network:                      #workload.spec.input.network
	}
	vAppName:  #workload.spec.input.vAppName
	clusterid: "NO_RDE_\(#workload.spec.input.clusterID)"
}

_configMap: corev1.#ConfigMap & {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: {
		name:      "vcloud-ccm-configmap"
		namespace: #workload.spec.targetNamespace
	}
	data: "vcloud-ccm-config.yaml": "\(yaml.Marshal(_config))"
}

_secret: corev1.#Secret & {
	apiVersion: "v1"
	kind:       "Secret"
	metadata: {
		name:      "vcloud-basic-auth"
		namespace: #workload.spec.targetNamespace
	}
	data: "refreshToken": '\(base64.Encode(null, #workload.spec.input.apiToken))'
}

_kustomization: kustomize.#Kustomization & {
	apiVersion: kustomize.#KustomizationVersion
	kind:       kustomize.#KustomizationKind
	resources: [
		"https://raw.githubusercontent.com/vmware/cloud-provider-for-cloud-director/refs/tags/1.6.1/manifests/cloud-director-ccm.yaml",
		"configmap.yaml",
		"secret.yaml",
	]
}

worktree: dockyardsv1.#Worktree & {
	apiVersion: "dockyards.io/v1alpha3"
	kind:       dockyardsv1.#WorktreeKind
	metadata: {
		name:      #workload.metadata.name
		namespace: #workload.metadata.namespace
	}
	spec: {
		files: {
			"configmap.yaml":     '\(yaml.Marshal(_configMap))'
			"secret.yaml":        '\(yaml.Marshal(_secret))'
			"kustomization.yaml": '\(yaml.Marshal(_kustomization))'
		}
	}
}

kustomization: kustomizev1.#Kustomization & {
	apiVersion: "kustomize.toolkit.fluxcd.io/v1"
	kind:       kustomizev1.#KustomizationKind
	metadata: {
		name:      #workload.metadata.name
		namespace: #workload.metadata.namespace
	}
	spec: {
		interval: "15m"
		kubeConfig: secretRef: name: #cluster.metadata.name + "-kubeconfig"
		prune:         true
		retryInterval: "30s"
		sourceRef: {
			kind: sourcev1.#GitRepositoryKind
			name: #workload.metadata.name
		}
		wait: true
	}
}
