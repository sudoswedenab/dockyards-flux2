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

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/parser"
	"github.com/spf13/pflag"
	dockyardsv1 "github.com/sudoswedenab/dockyards-backend/api/v1alpha3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func main() {
	var filename string
	var namespace string
	pflag.StringVar(&filename, "filename", "test.cue", "filename")
	pflag.StringVar(&namespace, "namespace", "dockyards", "namespace")
	pflag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	cfg, err := config.GetConfig()
	if err != nil {
		fmt.Println(err)

		os.Exit(1)
	}

	scheme := runtime.NewScheme()

	_ = dockyardsv1.AddToScheme(scheme)

	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		fmt.Println(err)

		os.Exit(1)
	}

	base := filepath.Base(filename)
	name := strings.TrimSuffix(base, ".cue")

	file, err := parser.ParseFile(filename, nil)
	if err != nil {
		fmt.Println(err)

		os.Exit(1)
	}

	b, err := format.Node(file, format.TabIndent(false), format.UseSpaces(2), format.Simplify())
	if err != nil {
		fmt.Println(err)

		os.Exit(1)
	}

	workloadTemplate := dockyardsv1.WorkloadTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}

	operationResult, err := controllerutil.CreateOrPatch(ctx, c, &workloadTemplate, func() error {
		workloadTemplate.Spec.Type = dockyardsv1.WorkloadTemplateTypeCue
		workloadTemplate.Spec.Source = string(b)

		return nil
	})
	if err != nil {
		fmt.Println(err)

		os.Exit(1)
	}

	fmt.Println("result:", operationResult)
}
