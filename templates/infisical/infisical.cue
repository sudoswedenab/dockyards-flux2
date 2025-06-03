package template

import (
    "encoding/yaml"
    "strings"
    "list"
    
    corev1 "k8s.io/api/core/v1"
    dockyardsv1 "bitbucket.org/sudosweden/dockyards-backend/pkg/api/v1alpha3"
    helmv2 "github.com/fluxcd/helm-controller/api/v2"
    sourcev1 "github.com/fluxcd/source-controller/api/v1"
    networkingv1 "k8s.io/api/networking/v1"
    apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
    kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
)

#Input: {
    repository:   string | *"https://dl.cloudsmith.io/public/infisical/helm-charts/helm/charts/"
    chart:        string | *"infisical-standalone"
    version:      string | *"1.5.0"
    ingressHost?: string & =~"^[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}$"
    storageClass: string | *"cephfs"
    storageSize:  string | *"2Gi"
}

#cluster: dockyardsv1.#Cluster

#workload: dockyardsv1.#Workload
#workload: spec: input: #Input

_namespace: corev1.#Namespace & {
    apiVersion: "v1"
    kind:       "Namespace"
    metadata: {
        name: string | *"infisical"  // Add default
        if #workload.spec.targetNamespace != _|_ {
            name: #workload.spec.targetNamespace
        }
        labels: {
            "pod-security.kubernetes.io/enforce":         "baseline"
            "pod-security.kubernetes.io/enforce-version": "latest"
        }
    }
}

worktree: dockyardsv1.#Worktree & {
    apiVersion: "dockyards.io/v1alpha3"
    kind:       dockyardsv1.#WorktreeKind
    metadata: {
        name: string | *"infisical"
        if #workload.metadata.name != _|_ {
            name: #workload.metadata.name
        }
        namespace: string | *"dockyards-system"
        if #workload.metadata.namespace != _|_ {
            namespace: #workload.metadata.namespace
        }
    }
    spec: files: {
        // Namespace resources
        "namespace/namespace.yaml": '\(yaml.Marshal(_namespace))'
        //"namespace/kustomization.yaml": '\(yaml.Marshal({
            //apiVersion: "kustomize.config.k8s.io/v1beta1"
            //kind:       "Kustomization"
            //resources: [
                //"namespace.yaml"
            //]
        //}))'

        // Workload resources
        "workload/secrets.yaml":   '\(yaml.Marshal(_infisicalSecret))'
        "workload/ingress.yaml":   '\(yaml.Marshal(_infisicalIngress))'
        //"workload/kustomization.yaml": '\(yaml.Marshal({
            //apiVersion: "kustomize.config.k8s.io/v1beta1"
            //kind:       "Kustomization"
            //namespace: string | *"infisical"  // Set default
            //if #workload.spec.targetNamespace != _|_ {
                //namespace: #workload.spec.targetNamespace
            //}
            //resources: [
                //"secrets.yaml",
                //"ingress.yaml"
            //]
        //}))'
    }
}

kustomization: kustomizev1.#Kustomization & {
    apiVersion: "kustomize.toolkit.fluxcd.io/v1"
    kind:       kustomizev1.#KustomizationKind
    metadata: {
        name:      #workload.metadata.name & !=""      // Ensure non-empty
        namespace: #workload.metadata.namespace & !=""  // Ensure non-empty
    }
    spec: {
        interval: "5m"
        kubeConfig: secretRef: name: #cluster.metadata.name + "-kubeconfig"
        prune:    true
        // Fix dependsOn reference
        dependsOn: [
            {
                name: "\(metadata.name)-namespace"  // Reference local metadata
            }
        ]
        path:     "./workload"
        sourceRef: {
            kind: sourcev1.#GitRepositoryKind
            name: worktree.metadata.name  // Reference local metadata
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

_values: apiextensionsv1.#JSON & {
    backend: {
        database: type: "postgresql"
        replicaCount: 2
    }
    ingress: {
        enabled: true
        nginx: enabled: false
    }
    mongodb: enabled: false
    postgresql: {
        enabled: true
        primary: persistence: {
            enabled:      true
            size:        #workload.spec.input.storageSize
            storageClass: #workload.spec.input.storageClass
        }
    }
    redis: master: persistence: {
        enabled:      true
        size:        #workload.spec.input.storageSize
        storageClass: #workload.spec.input.storageClass
    }
}

helmRelease: helmv2.#HelmRelease & {
    apiVersion: "helm.toolkit.fluxcd.io/v2"
    kind:       helmv2.#HelmReleaseKind
    metadata: {
        name:      #workload.metadata.name
        namespace: #workload.metadata.namespace  // FluxCD controller namespace
    }
    spec: {
        chart: spec: {
            chart:   #workload.spec.input.chart
            version: #workload.spec.input.version
            sourceRef: {
                kind: helmRepository.kind
                name: helmRepository.metadata.name
            }
        }
        interval:         "5m"
        install:         remediation: retries: -1
        kubeConfig:      secretRef: name: #cluster.metadata.name + "-kubeconfig"
        targetNamespace: string | *"infisical"  // Add default
        if #workload.spec.targetNamespace != _|_ {
            targetNamespace: #workload.spec.targetNamespace
        }
        storageNamespace: string | *"infisical"  // Add default
        if #workload.spec.targetNamespace != _|_ {
            storageNamespace: #workload.spec.targetNamespace
        }
        values: _values
    }
}

#generateSecret: {
    _chars: "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
    length: uint | *32
    _charList: strings.Split(_chars, "")
    value: strings.Join([
        for i in list.Range(0, length, 1) {
            _charList[i]
        }
    ], "")
}

_infisicalSecret: {
    apiVersion: "v1"
    kind:       "Secret"
    metadata: {
        name: string | *"infisical-secrets"
        namespace: string | *"infisical"
        if #workload.spec.targetNamespace != _|_ {
            namespace: #workload.spec.targetNamespace
        }
        labels: {
            "app.kubernetes.io/name":      "infisical"
            "app.kubernetes.io/component": "secrets"
            "dockyards.io/managed-by":     "flux"
        }
    }
    type: "Opaque"
    immutable: true

    _authSecret:       #generateSecret.value
    _encryptionSecret: #generateSecret.value

    stringData: {
        AUTH_SECRET:    _authSecret
        ENCRYPTION_KEY: _encryptionSecret

        if #workload.spec.input.ingressHost != _|_ {
            SITE_URL: "https://\(#workload.spec.input.ingressHost)"
        }
    }
}

_infisicalIngress: networkingv1.#Ingress & {
    apiVersion: "networking.k8s.io/v1"
    kind:       "Ingress"
    metadata: {
        name: string | *"infisical"
        namespace: string | *"infisical"
        if #workload.spec.targetNamespace != _|_ {
            namespace: #workload.spec.targetNamespace
        }
        annotations: {
            "nginx.ingress.kubernetes.io/rewrite-target": "/"
            "cert-manager.io/cluster-issuer":             "letsencrypt"
        }
    }
    spec: {
        ingressClassName: "nginx"
        tls: [...] | *[]
        rules: [...] | *[]
        if #workload.spec.input.ingressHost != _|_ {
            tls: [{
                hosts:      [#workload.spec.input.ingressHost]
                secretName: "infisical-cert"
            }]
            rules: [{
                host: #workload.spec.input.ingressHost
                http: {
                    paths: [{
                        path:     "/"
                        pathType: "Prefix"
                        backend: {
                            service: {
                                name: "\(#workload.metadata.name)-infisical-standalone-infisical"
                                port: number: 8080
                            }
                        }
                    }]
                }
            }]
        }
    }
}

// Add namespace kustomization
namespaceKustomization: kustomizev1.#Kustomization & {
    apiVersion: "kustomize.toolkit.fluxcd.io/v1"
    kind:       kustomizev1.#KustomizationKind
    metadata: {
        name:      "\(#workload.metadata.name)-namespace"
        namespace: #workload.metadata.namespace
    }
    spec: {
        interval: "5m"
        kubeConfig: secretRef: name: #cluster.metadata.name + "-kubeconfig"
        path:     "./namespace"
        prune:    true
        sourceRef: {
            kind: sourcev1.#GitRepositoryKind
            name: #workload.metadata.name
        }
    }
}

