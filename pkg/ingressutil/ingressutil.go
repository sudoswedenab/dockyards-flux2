package ingressutil

import (
	"net/url"

	networkingv1 "k8s.io/api/networking/v1"
)

func GetURLsFromIngress(ingress *networkingv1.Ingress) []string {
	urls := []string{}

	if len(ingress.Status.LoadBalancer.Ingress) == 0 {
		return urls
	}

	for _, tls := range ingress.Spec.TLS {
		for _, host := range tls.Hosts {
			u := url.URL{
				Scheme: "https",
				Host:   host,
			}

			urls = append(urls, u.String())
		}
	}

	if len(urls) > 0 {
		return urls
	}

	for _, rule := range ingress.Spec.Rules {
		u := url.URL{
			Scheme: "http",
			Host:   rule.Host,
		}

		urls = append(urls, u.String())
	}

	return urls
}
