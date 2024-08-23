package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"

	"bitbucket.org/sudosweden/dockyards-flux2/controllers"
	"github.com/go-logr/logr"
	"sigs.k8s.io/cluster-api/controllers/remote"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type warningLogr struct {
	logger *slog.Logger
}

func (l *warningLogr) HandleWarningHeader(_ int, _ string, msg string) {
	l.logger.Warn(msg)
}

func main() {
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

	mgr, err := ctrl.NewManager(cfg, manager.Options{})
	if err != nil {
		logger.Error("error creating manager", "err", err)

		os.Exit(1)
	}

	tracker, err := remote.NewClusterCacheTracker(mgr, remote.ClusterCacheTrackerOptions{})
	if err != nil {
		logger.Error("error creating new cluster cache tracker", "err", err)

		os.Exit(1)
	}

	err = (&controllers.HelmDeploymentReconciler{
		Client: mgr.GetClient(),
	}).SetupWithManager(mgr)
	if err != nil {
		logger.Error("error creating helm deployment reconciler", "err", err)

		os.Exit(1)
	}

	err = (&controllers.KustomizeDeploymentReconciler{
		Client: mgr.GetClient(),
	}).SetupWithManager(mgr)
	if err != nil {
		logger.Error("error creating kustomize deployment reconciler", "err", err)

		os.Exit(1)
	}

	err = (&controllers.ContainerImageDeploymentReconciler{
		Client: mgr.GetClient(),
	}).SetupWithManager(mgr)
	if err != nil {
		logger.Error("error creating container image deployment reconciler", "err", err)

		os.Exit(1)
	}

	err = (&controllers.HelmReleaseReconciler{
		Client:  mgr.GetClient(),
		Tracker: tracker,
	}).SetupWithManager(mgr)
	if err != nil {
		logger.Error("error creating helm release reconciler", "err", err)

		os.Exit(1)
	}

	err = mgr.Start(ctx)
	if err != nil {
		logger.Error("error starting manager", "err", err)

		os.Exit(1)
	}
}
