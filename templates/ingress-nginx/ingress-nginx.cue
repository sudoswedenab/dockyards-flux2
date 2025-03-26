package template

import (
	"encoding/yaml"

	dockyardsv1 "bitbucket.org/sudosweden/dockyards-backend/pkg/api/v1alpha3"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	corev1 "k8s.io/api/core/v1"
)

#Input: {
	url:  string | *"https://github.com/kubernetes/ingress-nginx"
	path: string | *"deploy/static/provider/cloud"
	tag:  string | *"controller-v1.12.1"
	service?: {
		annotations?: {
			[string]: string
		}
		loadBalancerIP?: string
	}
	isDefaultClass: bool | *true
	enableSSLPassThrough: bool | *true
}

#cluster: dockyardsv1.#Cluster

#workload: dockyardsv1.#Workload
#workload: spec: input: #Input

gitRepository: sourcev1.#GitRepository & {
	apiVersion: "source.toolkit.fluxcd.io/v1"
	kind:       sourcev1.#GitRepositoryKind
	metadata: {
		name:      #workload.metadata.name
		namespace: #workload.metadata.namespace
	}
	spec: {
		interval: "60m"
		url:      #workload.spec.input.url
		ref: {
			tag: #workload.spec.input.tag
		}
	}
}

_service: corev1.#Service & {
	apiVersion: "v1"
	kind:       "Service"
	metadata: {
		if #workload.spec.input.service.annotations != _|_ {
			annotations: #workload.spec.input.service.annotations
		}
		name: "ingress-nginx-controller"
	}
	if #workload.spec.input.service.loadBalancerIP != _|_ {
		spec: loadBalancerIP: #workload.spec.input.service.loadBalancerIP
	}
}

_patches: [
	if #workload.spec.input.service != _|_ {
		patch: "\(yaml.Marshal(_service))"
		target: {
			kind: "Service"
			name: "ingress-nginx-controller"
		}
	},
	{
		patch: """
			- op: replace
			  path: /kind
			  value: "DaemonSet"
			"""
		target: {
			kind:          "Deployment"
			labelSelector: "app.kubernetes.io/component=controller"
		}
	},
	if #workload.spec.input.enableSSLPassThrough {
		patch: """
			- op: add
			  path: /spec/template/spec/containers/0/args/-
			  value: "--enable-ssl-passthrough=true"
			"""
		target: {
			kind:          "DaemonSet"
			labelSelector: "app.kubernetes.io/component=controller"
		}
	},
	if #workload.spec.input.isDefaultClass {
		patch: """
			- op: add
			  path: /metadata/annotations/ingressclass.kubernetes.io~1is-default-class
			  value: "true"
			"""
		target: {
			kind: "IngressClass"
			name: "nginx"
		}
	},
]

kustomization: kustomizev1.#Kustomization & {
	apiVersion: "kustomize.toolkit.fluxcd.io/v1"
	kind:       kustomizev1.#KustomizationKind
	metadata: {
		name:      #workload.metadata.name
		namespace: #workload.metadata.namespace
	}
	spec: {
		force:    true
		interval: "15m"
		kubeConfig: secretRef: name: #cluster.metadata.name + "-kubeconfig"
		patches:       _patches
		prune:         true
		path:          #workload.spec.input.path
		retryInterval: "60s"
		sourceRef: {
			kind: gitRepository.kind
			name: gitRepository.metadata.name
		}
		targetNamespace: #workload.spec.targetNamespace
		wait:            true
		commonMetadata: labels: "dockyards.io/workload-name": #workload.metadata.name
	}
}
