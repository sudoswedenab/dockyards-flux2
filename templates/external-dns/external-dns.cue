package template

import (
	"encoding/yaml"
	"encoding/base64"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	dockyardsv1 "bitbucket.org/sudosweden/dockyards-backend/pkg/api/v1alpha3"
	kustomize "sigs.k8s.io/kustomize/api/types"
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
)

#Input: {
	ref:       string | *"70e32ad8d0f7c677cb4e2f9f8395ac9eaf5853ec"
	provider!: string
	credentials: [string]: string | *[]
	env: [string]: string | *[]
	sources: [...string] | *["ingress", "service"]
	zoneIDFilters?: [...string]
}

#cluster: dockyardsv1.#Cluster

#workload: dockyardsv1.#Workload
#workload: spec: input: #Input

_namespace: corev1.#Namespace & {
	apiVersion: "v1"
	kind:       "Namespace"
	metadata: name: #workload.spec.targetNamespace
}

_deployment: appsv1.#Deployment & {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: name: "external-dns"
	spec: template: spec: {
		containers: [
			{
				args: [
					"--provider=\(#workload.spec.input.provider)",
					for s in #workload.spec.input.sources {
						"--source=\(s)"
					},
					"--events",
					if #workload.spec.input.zoneIDFilters != _|_ {
						for zoneID in #workload.spec.input.zoneIDFilters {
							"--zone-id-filter=\(zoneID)"
						}
					},
				]
				env: [
					for k, v in #workload.spec.input.env {
						name:  k
						value: v
					},
				]
				name: "external-dns"
				volumeMounts: [
					{
						name:      "provider-credentials"
						mountPath: "/.provider"
						readOnly:  true
					},
				]
			},
		]
		volumes: [
			{
				name: "provider-credentials"
				secret: secretName: "external-dns"
			},
		]
	}
}

_patches: [
	{
		patch: "\(yaml.Marshal(_deployment))"
		target: {
			kind: "Deployment"
			name: "external-dns"
		}

	},
]

_secret: corev1.#Secret & {
	apiVersion: "v1"
	kind:       "Secret"
	metadata: name: "external-dns"
	data: {
		for k, v in #workload.spec.input.credentials {
			"\(k)": '\(base64.Encode(null, v))'
		}
	}
}

_kustomization: kustomize.#Kustomization & {
	apiVersion: "kustomize.config.k8s.io/v1beta1"
	kind:       kustomize.#KustomizationKind
	patches:    _patches
	resources: [
		"github.com/kubernetes-sigs/external-dns/kustomize?ref=\(#workload.spec.input.ref)",
		"secret.yaml",
		"namespace.yaml",
	]
}

worktree: dockyardsv1.#Worktree & {
	apiVersion: "dockyards.io/v1alpha3"
	kind:       dockyardsv1.#WorktreeKind
	metadata: {
		name:      #workload.metadata.name
		namespace: #workload.metadata.namespace
	}
	spec: files: {
		"kustomization.yaml": '\(yaml.Marshal(_kustomization))'
		"secret.yaml":        '\(yaml.Marshal(_secret))'
		"namespace.yaml":     '\(yaml.Marshal(_namespace))'
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
		retryInterval: "60s"
		sourceRef: {
			kind: sourcev1.#GitRepositoryKind
			name: worktree.metadata.name
		}
		targetNamespace: #workload.spec.targetNamespace
		wait:            true
	}
}
