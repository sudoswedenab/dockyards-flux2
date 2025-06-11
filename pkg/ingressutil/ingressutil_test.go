package ingressutil_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sudoswedenab/dockyards-flux2/pkg/ingressutil"
	networkingv1 "k8s.io/api/networking/v1"
)

func TestGetURLsFromIngress(t *testing.T) {
	tt := []struct {
		name     string
		ingress  networkingv1.Ingress
		expected []string
	}{
		{
			name: "test simple ingress",
			ingress: networkingv1.Ingress{
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host: "test.dockyards.dev",
						},
					},
				},
				Status: networkingv1.IngressStatus{
					LoadBalancer: networkingv1.IngressLoadBalancerStatus{
						Ingress: []networkingv1.IngressLoadBalancerIngress{
							{
								IP: "1.2.3.4",
							},
						},
					},
				},
			},
			expected: []string{
				"http://test.dockyards.dev",
			},
		},
		{
			name: "test ingress with tls",
			ingress: networkingv1.Ingress{
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host: "test.dockyards.dev",
						},
					},
					TLS: []networkingv1.IngressTLS{
						{
							Hosts: []string{

								"test.dockyards.dev",
							},
						},
					},
				},
				Status: networkingv1.IngressStatus{
					LoadBalancer: networkingv1.IngressLoadBalancerStatus{
						Ingress: []networkingv1.IngressLoadBalancerIngress{
							{
								IP: "1.2.3.4",
							},
						},
					},
				},
			},
			expected: []string{
				"https://test.dockyards.dev",
			},
		},
		{
			name: "test ingress without status",
			ingress: networkingv1.Ingress{
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host: "test.dockyards.dev",
						},
					},
				},
			},
			expected: []string{},
		},
		{
			name: "test ingress without ip",
			ingress: networkingv1.Ingress{
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host: "test.dockyards.dev",
						},
					},
				},
				Status: networkingv1.IngressStatus{
					LoadBalancer: networkingv1.IngressLoadBalancerStatus{
						Ingress: []networkingv1.IngressLoadBalancerIngress{},
					},
				},
			},
			expected: []string{},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			actual := ingressutil.GetURLsFromIngress(&tc.ingress)
			if !cmp.Equal(actual, tc.expected) {
				t.Errorf("diff: %s", cmp.Diff(tc.expected, actual))
			}
		})
	}
}
