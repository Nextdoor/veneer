/*
Copyright 2025 Veneer Contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Main entrypoint for the Veneer controller manager.
// This file is scaffolded from kubebuilder and will be tested through E2E tests, not unit tests.
//
// Coverage: Excluded - main entrypoints are tested via E2E tests

package main

import (
	"flag"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	karpenterv1alpha1 "sigs.k8s.io/karpenter/pkg/apis/v1alpha1"

	"github.com/nextdoor/veneer/pkg/config"
	"github.com/nextdoor/veneer/pkg/overlay"
	"github.com/nextdoor/veneer/pkg/prometheus"
	"github.com/nextdoor/veneer/pkg/reconciler"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	// Register Karpenter v1alpha1 types (NodeOverlay)
	// Karpenter doesn't export an AddToScheme function, so we register types directly
	karpenterGV := schema.GroupVersion{Group: "karpenter.sh", Version: "v1alpha1"}
	scheme.AddKnownTypes(karpenterGV,
		&karpenterv1alpha1.NodeOverlay{},
		&karpenterv1alpha1.NodeOverlayList{},
	)
	metav1.AddToGroupVersion(scheme, karpenterGV)

	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var configFile string
	var overlayDisabled bool

	flag.StringVar(&configFile, "config", "/etc/veneer/config.yaml",
		"Path to the controller configuration file. Can be overridden with VENEER_CONFIG_PATH environment variable.")
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080",
		"The address the metrics endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081",
		"The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&overlayDisabled, "overlay-disabled", false,
		"Create NodeOverlays in disabled mode (with impossible requirements). "+
			"Overlays will be created but won't affect Karpenter provisioning decisions. "+
			"Can also be set via config file (overlays.disabled) or VENEER_OVERLAY_DISABLED env var.")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Allow environment variable to override config file path
	if envConfigPath := os.Getenv("VENEER_CONFIG_PATH"); envConfigPath != "" {
		configFile = envConfigPath
	}

	// Load controller configuration
	cfg, err := config.Load(configFile)
	if err != nil {
		if _, statErr := os.Stat(configFile); os.IsNotExist(statErr) {
			setupLog.Info("config file not found, using defaults", "config-file", configFile)
			cfg = &config.Config{}
		} else {
			setupLog.Error(err, "failed to load configuration", "config-file", configFile)
			os.Exit(1)
		}
	} else {
		setupLog.Info("loaded configuration",
			"prometheus-url", cfg.PrometheusURL,
			"log-level", cfg.LogLevel)
	}

	// CLI flag overrides config file for overlay-disabled
	if overlayDisabled {
		cfg.Overlays.Disabled = true
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "veneer.nextdoor.com",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// +kubebuilder:scaffold:builder

	// Create Prometheus client for querying Lumina metrics
	promClient, err := prometheus.NewClient(
		cfg.PrometheusURL,
		cfg.AWS.AccountID,
		cfg.AWS.Region,
		setupLog.WithName("prometheus-client"),
	)
	if err != nil {
		setupLog.Error(err, "unable to create Prometheus client", "url", cfg.PrometheusURL)
		os.Exit(1)
	}

	// Create decision engine and generator for NodeOverlay lifecycle management
	decisionEngine := overlay.NewDecisionEngine(cfg)
	generator := overlay.NewGeneratorWithOptions(cfg.Overlays.Disabled)

	// Log disabled mode status at startup
	if cfg.Overlays.Disabled {
		setupLog.Info("overlay disabled mode enabled - NodeOverlays will be created with impossible requirements")
	}

	// Create and start metrics reconciler
	metricsReconciler := &reconciler.MetricsReconciler{
		PrometheusClient: promClient,
		Config:           cfg,
		DecisionEngine:   decisionEngine,
		Generator:        generator,
		Logger:           ctrl.Log.WithName("metrics-reconciler"),
		Client:           mgr.GetClient(),
		// Use default 5 minute interval
	}

	// Add metrics reconciler as a runnable
	if err := mgr.Add(metricsReconciler); err != nil {
		setupLog.Error(err, "unable to add metrics reconciler to manager")
		os.Exit(1)
	}

	// Setup health checks
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
