package template

import (
  "encoding/yaml"

  corev1 "k8s.io/api/core/v1"
  appsv1 "k8s.io/api/apps/v1"
  dockyardsv1 "bitbucket.org/sudosweden/dockyards-backend/pkg/api/v1alpha3"
  helmv2 "github.com/fluxcd/helm-controller/api/v2"
  sourcev1 "github.com/fluxcd/source-controller/api/v1"
  kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
)

// --- Parameters for Agent Deployment ---
#Input: {
  namespace:      string | *"infisical"
  agentName:      string | *"infisical-agent"
  image:          string | *"infisical/agent:latest"
  replicas:       int | *1
  configFileName: string | *"infisical-agent.yaml"
  config:         string // YAML string for agent config
}

#cluster: dockyardsv1.#Cluster

#workload: dockyardsv1.#Workload
#workload: spec: input: #Input

// --- Namespace Resource ---
_namespace: corev1.#Namespace & {
  apiVersion: "v1"
  kind:       "Namespace"
  metadata: {
    name: #workload.spec.input.namespace
    labels: {
      "pod-security.kubernetes.io/enforce":         "baseline"
      "pod-security.kubernetes.io/enforce-version": "latest"
    }
  }
}

// --- ConfigMap for Agent Config ---
_configMap: corev1.#ConfigMap & {
  apiVersion: "v1"
  kind:       "ConfigMap"
  metadata: {
    name:      #workload.spec.input.agentName + "-config"
    namespace: #workload.spec.input.namespace
  }
  data: {
    "\(#workload.spec.input.configFileName)": #workload.spec.input.config
  }
}

// --- ServiceAccount for Agent ---
_serviceAccount: corev1.#ServiceAccount & {
  apiVersion: "v1"
  kind:       "ServiceAccount"
  metadata: {
    name:      #workload.spec.input.agentName
    namespace: #workload.spec.input.namespace
  }
}

// --- Agent Deployment ---
_deployment: appsv1.#Deployment & {
  apiVersion: "apps/v1"
  kind:       "Deployment"
  metadata: {
    name:      #workload.spec.input.agentName
    namespace: #workload.spec.input.namespace
    labels: {
      "app.kubernetes.io/name": #workload.spec.input.agentName
    }
  }
  spec: {
    replicas: #workload.spec.input.replicas
    selector: matchLabels: {
      "app.kubernetes.io/name": #workload.spec.input.agentName
    }
    template: {
      metadata: labels: {
        "app.kubernetes.io/name": #workload.spec.input.agentName
      }
      spec: {
        serviceAccountName: #workload.spec.input.agentName
        containers: [{
          name:  #workload.spec.input.agentName
          image: #workload.spec.input.image
          volumeMounts: [{
            name:      "config"
            mountPath: "/etc/infisical"
          }]
          args: [
            "--config",
            "/etc/infisical/\(#workload.spec.input.configFileName)",
          ]
        }]
        volumes: [{
          name: "config"
          configMap: name: #workload.spec.input.agentName + "-config"
        }]
      }
    }
  }
}

// --- Output Files ---
worktree: dockyardsv1.#Worktree & {
  apiVersion: "dockyards.io/v1alpha3"
  kind:       dockyardsv1.#WorktreeKind
  metadata: {
    name:      #workload.metadata.name
    namespace: #workload.metadata.namespace
  }
  spec: files: {
    "namespace.yaml":       '\(yaml.Marshal(_namespace))'
    "configmap.yaml":       '\(yaml.Marshal(_configMap))'
    "serviceaccount.yaml": '\(yaml.Marshal(_serviceAccount))'
    "deployment.yaml":      '\(yaml.Marshal(_deployment))'
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
