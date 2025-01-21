package template

import (
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	dockyardsv1 "bitbucket.org/sudosweden/dockyards-backend/pkg/api/v1alpha3"
	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

#_domain: =~"^[a-z.-].*$"

#Input: {
	domain!:          string & #_domain
	repository:       string | *"https://argoproj.github.io/argo-helm"
	chart:            string | *"argo-cd"
	version:          string | *"7.6.12"
	ingressClassName: string | *"nginx"
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
		interval: "1h"
		url:      #workload.spec.input.repository
	}
}

_values: apiextensionsv1.#JSON & {
	global: {
		domain: #workload.spec.input.domain
	}
	server: {
		ingress: {
			if #workload.spec.input.ingressClassName == "nginx" {
				annotations: {
					"nginx.ingress.kubernetes.io/force-ssl-redirect": "true"
					"nginx.ingress.kubernetes.io/ssl-passthrough":    "true"
				}
			}
			enabled: true
			tls:     true
		}
	}
	certificate: {
		enabled: true
		issuer: {
			kind: "ClusterIssuer"
			name: "letsencrypt"
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
		chart: {
			spec: {
				chart: #workload.spec.input.chart
				sourceRef: {
					kind: helmRepository.kind
					name: helmRepository.metadata.name
				}
				version: #workload.spec.input.version
			}
		}
		interval: "5m"
		install: {
			createNamespace: true
			remediation: retries: -1
		}
		kubeConfig: {
			secretRef: {
				name: #cluster.metadata.name + "-kubeconfig"
			}
		}
		storageNamespace: #workload.spec.targetNamespace
		targetNamespace:  #workload.spec.targetNamespace
		values:           _values
	}
}
