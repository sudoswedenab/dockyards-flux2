package ingressutil_test

import (
	"testing"

	"bitbucket.org/sudosweden/dockyards-flux2/pkg/ingressutil"
	"github.com/google/go-cmp/cmp"
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
			},
			expected: []string{
				"https://test.dockyards.dev",
			},
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
