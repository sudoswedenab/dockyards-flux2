package main

//go:generate go run sigs.k8s.io/controller-tools/cmd/controller-gen rbac:roleName=dockyards-flux2 webhook paths="./..."
//go:generate go run bitbucket.org/sudosweden/dockyards-flux2/cmd/generate-workloadtemplate-manifests
