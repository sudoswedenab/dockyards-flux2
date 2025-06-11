// Copyright 2025 Sudo Sweden AB
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
