package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"

	"github.com/go-logr/logr"
	"github.com/spf13/pflag"
	dockyardsv1 "github.com/sudoswedenab/dockyards-backend/api/v1alpha3"
	"github.com/sudoswedenab/dockyards-backend/api/v1alpha3/index"
	"github.com/sudoswedenab/dockyards-flux2/controllers"
	"github.com/sudoswedenab/dockyards-flux2/webhooks"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

type warningLogr struct {
	logger *slog.Logger
}

func (l *warningLogr) HandleWarningHeader(_ int, _ string, msg string) {
	l.logger.Warn(msg)
}

func main() {
	var metricsBindAddress string
	var enableWebhooks bool
	pflag.StringVar(&metricsBindAddress, "metrics-bind-address", "0", "metrics bind address")
	pflag.BoolVar(&enableWebhooks, "enable-webhooks", false, "enable webhooks")
	pflag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slogr := logr.FromSlogHandler(logger.Handler())

	ctrl.SetLogger(slogr)

	cfg, err := config.GetConfig()
	if err != nil {
		logger.Error("error getting config", "err", err)

		os.Exit(1)
	}

	w := warningLogr{
		logger: logger,
	}

	cfg.WarningHandler = &w

	scheme := runtime.NewScheme()

	_ = dockyardsv1.AddToScheme(scheme)

	options := manager.Options{
		Metrics: metricsserver.Options{
			BindAddress: metricsBindAddress,
		},
		Scheme: scheme,
	}

	mgr, err := ctrl.NewManager(cfg, options)
	if err != nil {
		logger.Error("error creating manager", "err", err)

		os.Exit(1)
	}

	err = mgr.GetFieldIndexer().IndexField(ctx, &dockyardsv1.Workload{}, index.WorkloadTemplateReferenceField, index.ByWorkloadTemplateReference)
	if err != nil {
		logger.Error("error adding index for workload", "err", err)

		os.Exit(1)
	}

	err = (&controllers.DockyardsWorkloadReconciler{
		Client: mgr.GetClient(),
	}).SetupWithManager(mgr)
	if err != nil {
		logger.Error("error creating workload reconciler", "err", err)

		os.Exit(1)
	}

	err = (&controllers.DockyardsWorktreeReconciler{
		Client: mgr.GetClient(),
	}).SetupWithManager(mgr)
	if err != nil {
		logger.Error("error creating worktree reconciler", "err", err)

		os.Exit(1)
	}

	err = (&controllers.KustomizationReconciler{
		Client: mgr.GetClient(),
	}).SetupWithManager(mgr)
	if err != nil {
		logger.Error("error creating kustomization reconciler", "err", err)

		os.Exit(1)
	}

	err = (&controllers.HelmReleaseReconciler{
		Client: mgr.GetClient(),
	}).SetupWithManager(mgr)
	if err != nil {
		logger.Error("error creating helm release reconciler", "err", err)

		os.Exit(1)
	}

	if enableWebhooks {
		err := (&webhooks.DockyardsWorkloadTemplate{}).SetupWithManager(mgr)
		if err != nil {
			logger.Error("error creating workload template webhooks", "err", err)

			os.Exit(1)
		}

		err = (&webhooks.DockyardsWorkload{
			Client: mgr.GetClient(),
		}).SetupWithManager(mgr)
		if err != nil {
			logger.Error("error creating workload template webhooks", "err", err)

			os.Exit(1)
		}
	}

	err = mgr.Start(ctx)
	if err != nil {
		logger.Error("error starting manager", "err", err)

		os.Exit(1)
	}
}
