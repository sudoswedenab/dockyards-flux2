package template

import (
	"encoding/yaml"

	dockyardsv1 "bitbucket.org/sudosweden/dockyards-backend/pkg/api/v1alpha3"
	appsv1 "k8s.io/api/apps/v1"
	kustomize "sigs.k8s.io/kustomize/api/types"
	corev1 "k8s.io/api/core/v1"
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	networkingv1 "k8s.io/api/networking/v1"
)

#Input: {
	image!:   string & =~"^[a-z0-9]+((\\.|_|__|-+)[a-z0-9]+)*(\/[a-z0-9]+((\\.|_|__|-+)[a-z0-9]+)*)*$"
	tag:      string & =~"^[a-zA-Z0-9_][a-zA-Z0-9._-]{0,127}$" | *"latest"
	replicas: uint | *1
	port?:    uint16
	host?:    string
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
	metadata: {
		name:      #workload.metadata.name
		namespace: _namespace.metadata.name
	}
	spec: {
		replicas: #workload.spec.input.replicas
		selector: matchLabels: "app.kubernetes.io/name": #workload.metadata.name
		template: {
			metadata: labels: "app.kubernetes.io/name": #workload.metadata.name
			spec: {
				containers: [
					{
						name:  #workload.metadata.name
						image: "\(#workload.spec.input.image):\(#workload.spec.input.tag)"
						if #workload.spec.input.port != _|_ {
							ports: [
								{
									containerPort: #workload.spec.input.port
								},
							]
						}
					},
				]
			}
		}
	}
}

_service: corev1.#Service & {
	apiVersion: "v1"
	kind:       "Service"
	metadata: {
		name:      #workload.metadata.name
		namespace: _namespace.metadata.name
	}
	spec: {
		ports: [
			{
				port:       #workload.spec.input.port
				targetPort: #workload.spec.input.port
			},
		]
		selector: {
			"app.kubernetes.io/name": #workload.metadata.name
		}
	}
}

_ingress: networkingv1.#Ingress & {
	apiVersion: "networking.k8s.io/v1"
	kind:       "Ingress"
	metadata: {
		name:      #workload.metadata.name
		namespace: _namespace.metadata.name
	}
	spec: {
		rules: [
			{
				host: #workload.spec.input.host
				http: {
					paths: [
						{
							path:     "/"
							pathType: networkingv1.#PathTypePrefix
							backend: {
								service: {
									name: #workload.metadata.name
									port: number: #workload.spec.input.port
								}
							}
						},
					]
				}
			},
		]
	}
}

_resources: {
	"namespace.yaml":  '\(yaml.Marshal(_namespace))'
	"deployment.yaml": '\(yaml.Marshal(_deployment))'
}

if #workload.spec.input.port != _|_ {
	_resources: "service.yaml": '\(yaml.Marshal(_service))'

	if #workload.spec.input.host != _|_ {
		_resources: "ingress.yaml": '\(yaml.Marshal(_ingress))'
	}
}

_kustomization: kustomize.#Kustomization & {
	resources: [
		for key, val in _resources {
			"\(key)"
		},
	]
}

worktree: dockyardsv1.#Worktree & {
	apiVersion: "dockyards.io/v1alpha3"
	kind:       dockyardsv1.#WorktreeKind
	metadata: {
		name:      #workload.metadata.name
		namespace: #workload.metadata.namespace
	}
	spec: files: "kustomization.yaml": '\(yaml.Marshal(_kustomization))'
	spec: files: {
		for key, val in _resources {
			"\(key)": val
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
		interval: "5m"
		kubeConfig: secretRef: name: #cluster.metadata.name + "-kubeconfig"
		prune: true
		sourceRef: {
			kind: sourcev1.#GitRepositoryKind
			name: worktree.metadata.name
		}
		targetNamespace: _namespace.metadata.name
	}
}
