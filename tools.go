//go:build tools

// +tools

package main

import (
	_ "sigs.k8s.io/controller-tools/cmd/controller-gen"
	_ "sigs.k8s.io/kustomize/api/types"
	_ "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	_ "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
)
