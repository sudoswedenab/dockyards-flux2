package template

import (
	"encoding/yaml"
	"encoding/base64"
	"strings"

	corev1 "k8s.io/api/core/v1"
	dockyardsv1 "bitbucket.org/sudosweden/dockyards-backend/pkg/api/v1alpha3"
	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
)

#RemoteWrite: {
	basicAuth?: {
		url!:        string
		secretName!: string
		username!:   string
		password!:   string
	}
}

#Input: {
	repository: string | *"https://prometheus-community.github.io/helm-charts"
	chart:      string | *"kube-prometheus-stack"
	version:    string | *"67.11.0"
	agentMode:  bool | *true
	remoteWrite?: [...#RemoteWrite]
}

_secretList: [
	for remoteWriteConfig in #workload.spec.input.remoteWrite {
		corev1.#Secret
		apiVersion: "v1"
		kind:       "Secret"
		metadata: {
			name:      remoteWriteConfig.basicAuth.secretName
			namespace: #workload.spec.targetNamespace
		}
		data: {
			"username": '\(base64.Encode(null, remoteWriteConfig.basicAuth.username))'
			"password": '\(base64.Encode(null, remoteWriteConfig.basicAuth.password))'
		}
	},
]

_values: apiextensionsv1.#JSON & {
	alertmanager: enabled: false
	grafana: enabled:      false
	prometheus: {
		agentMode: #workload.spec.input.agentMode
		prometheusSpec: {
			scrapeInterval:                      "30s"
			evaluationInterval:                  "30s"
			podMonitorSelectorNilUsesHelmValues: false
			podMonitorNamespaceSelector: {}
			if #workload.spec.input.remoteWrite != _|_ {
				remoteWrite: [
					for remoteWriteConfig in #workload.spec.input.remoteWrite {
						url: remoteWriteConfig.basicAuth.url
						basicAuth: {
							username: {
								name: remoteWriteConfig.basicAuth.secretName
								key:  "username"
							}
							password: {
								name: remoteWriteConfig.basicAuth.secretName
								key:  "password"
							}
						}
					},
				]
			}
		}
	}
}

#cluster: dockyardsv1.#Cluster

#workload: dockyardsv1.#Workload
#workload: spec: input: #Input

_namespace: corev1.#Namespace & {
	apiVersion: "v1"
	kind:       "Namespace"
	metadata: {
		name: #workload.spec.targetNamespace
		labels: {
			"pod-security.kubernetes.io/enforce":         "privileged"
			"pod-security.kubernetes.io/enforce-version": "latest"
		}
	}
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
			"namespace.yaml": '\(yaml.Marshal(_namespace))'
			"secrets.yaml":   '\(strings.Join([
						for secret in _secretList {
					"\(yaml.Marshal(secret))"
				},
			], "\n---\n"))'
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
			name: #workload.metadata.name
		}
	}
}

helmRepository: sourcev1.#HelmRepository & {
	apiVersion: "source.toolkit.fluxcd.io/v1"
	kind:       sourcev1.#HelmRepositoryKind
	metadata: {
		name:      #workload.metadata.name
		namespace: #workload.metadata.namespace
	}
	spec: {
		url:      #workload.spec.input.repository
		interval: "5m"
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
		chart: {
			spec: {
				chart:   #workload.spec.input.chart
				version: #workload.spec.input.version
				sourceRef: {
					kind: helmRepository.kind
					name: helmRepository.metadata.name
				}
			}
		}
		install: remediation: retries: -1
		interval: "5m"
		kubeConfig: secretRef: name: #cluster.metadata.name + "-kubeconfig"
		storageNamespace: #workload.spec.targetNamespace
		targetNamespace:  #workload.spec.targetNamespace
		values:           _values
	}
}
