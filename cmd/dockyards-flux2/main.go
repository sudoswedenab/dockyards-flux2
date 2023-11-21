package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"

	"bitbucket.org/sudosweden/dockyards-backend/pkg/api/v1alpha1"
	"bitbucket.org/sudosweden/dockyards-flux2/controller"
	"github.com/fluxcd/helm-controller/api/v2beta1"
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	"github.com/fluxcd/source-controller/api/v1"
	"github.com/fluxcd/source-controller/api/v1beta2"
	"github.com/go-logr/logr/slogr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	logr := slogr.NewLogr(logger.Handler())

	ctrl.SetLogger(logr)

	cfg, err := config.GetConfig()
	if err != nil {
		logger.Error("error getting config", "err", err)

		os.Exit(1)
	}

	manager, err := ctrl.NewManager(cfg, manager.Options{})
	if err != nil {
		logger.Error("error creating manager", "err", err)

		os.Exit(1)
	}

	scheme := manager.GetScheme()
	v1alpha1.AddToScheme(scheme)
	v2beta1.AddToScheme(scheme)
	v1beta2.AddToScheme(scheme)
	v1.AddToScheme(scheme)
	kustomizev1.AddToScheme(scheme)

	err = (&controller.HelmDeploymentReconciler{
		Client: manager.GetClient(),
		Logger: logger,
	}).SetupWithManager(manager)
	if err != nil {
		logger.Error("error creating helm deployment reconciler", "err", err)

		os.Exit(1)
	}

	err = (&controller.KustomizeDeploymentReconciler{
		Client: manager.GetClient(),
		Logger: logger,
	}).SetupWithManager(manager)
	if err != nil {
		logger.Error("error creating kustomize deployment reconciler", "err", err)

		os.Exit(1)
	}

	err = (&controller.ContainerImageDeploymentReconciler{
		Client: manager.GetClient(),
		Logger: logger,
	}).SetupWithManager(manager)
	if err != nil {
		logger.Error("error creating container image deployment reconciler", "err", err)

		os.Exit(1)
	}

	err = manager.Start(ctx)
	if err != nil {
		logger.Error("error starting manager", "err", err)

		os.Exit(1)
	}
}
