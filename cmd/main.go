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
	"context"
	"flag"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	karpenterv1alpha1 "sigs.k8s.io/karpenter/pkg/apis/v1alpha1"

	"github.com/nextdoor/veneer/pkg/config"
	"github.com/nextdoor/veneer/pkg/metrics"
	"github.com/nextdoor/veneer/pkg/overlay"
	"github.com/nextdoor/veneer/pkg/preference"
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
	karpenterv1alpha1GV := schema.GroupVersion{Group: "karpenter.sh", Version: "v1alpha1"}
	scheme.AddKnownTypes(karpenterv1alpha1GV,
		&karpenterv1alpha1.NodeOverlay{},
		&karpenterv1alpha1.NodeOverlayList{},
	)
	metav1.AddToGroupVersion(scheme, karpenterv1alpha1GV)

	// Register Karpenter v1 types (NodePool) for preference-based overlays
	karpenterv1GV := schema.GroupVersion{Group: "karpenter.sh", Version: "v1"}
	scheme.AddKnownTypes(karpenterv1GV,
		&karpenterv1.NodePool{},
		&karpenterv1.NodePoolList{},
	)
	metav1.AddToGroupVersion(scheme, karpenterv1GV)

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

	// Initialize Prometheus metrics using struct-based pattern.
	// Metrics are registered with the controller-runtime registry and exposed
	// via the /metrics endpoint.
	veneerMetrics := metrics.NewMetrics(ctrlmetrics.Registry)
	veneerMetrics.SetConfigMetrics(cfg.Overlays.Disabled, cfg.Overlays.UtilizationThreshold)
	setupLog.Info("metrics initialized")

	// Discover the controller's Deployment for owner reference on NodeOverlays.
	// This ensures all created overlays are garbage collected when the controller is uninstalled.
	// Uses POD_NAMESPACE and POD_NAME environment variables (set via Downward API in Helm chart).
	controllerRef := discoverControllerDeployment(context.Background(), mgr.GetClient(), setupLog)
	if controllerRef != nil {
		setupLog.Info("controller owner reference configured for NodeOverlay garbage collection")
	}

	// Create and start metrics reconciler
	metricsReconciler := &reconciler.MetricsReconciler{
		PrometheusClient: promClient,
		Config:           cfg,
		DecisionEngine:   decisionEngine,
		Generator:        generator,
		Logger:           ctrl.Log.WithName("metrics-reconciler"),
		Client:           mgr.GetClient(),
		Metrics:          veneerMetrics,
		ControllerRef:    controllerRef,
		// Use default 5 minute interval
	}

	// Add metrics reconciler as a runnable
	if err := mgr.Add(metricsReconciler); err != nil {
		setupLog.Error(err, "unable to add metrics reconciler to manager")
		os.Exit(1)
	}

	// Create and setup NodePool reconciler for preference-based overlays
	// This watches NodePools and generates NodeOverlays from veneer.io/preference.N annotations
	preferenceGenerator := preference.NewGeneratorWithOptions(cfg.Overlays.Disabled)
	nodePoolReconciler := &reconciler.NodePoolReconciler{
		Client:        mgr.GetClient(),
		Logger:        ctrl.Log.WithName("nodepool-reconciler"),
		Generator:     preferenceGenerator,
		Metrics:       veneerMetrics,
		ControllerRef: controllerRef,
	}
	if err := nodePoolReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to setup NodePool reconciler")
		os.Exit(1)
	}
	setupLog.Info("NodePool reconciler configured for preference-based overlays")

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

// discoverControllerDeployment discovers the Deployment that owns this controller pod.
// It uses the POD_NAMESPACE and POD_NAME environment variables (set via Downward API)
// to find the pod, then traverses owner references to find the Deployment.
//
// Returns nil if the environment variables are not set or the Deployment cannot be found.
// This allows the controller to run in environments without these variables (e.g., local development).
func discoverControllerDeployment(ctx context.Context, k8sClient client.Client, log logr.Logger) *metav1.OwnerReference {
	podNamespace := os.Getenv("POD_NAMESPACE")
	podName := os.Getenv("POD_NAME")

	if podNamespace == "" || podName == "" {
		log.Info("POD_NAMESPACE or POD_NAME not set, skipping controller deployment discovery")
		return nil
	}

	// Get the pod
	var pod corev1.Pod
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: podNamespace, Name: podName}, &pod); err != nil {
		log.Error(err, "Failed to get controller pod", "namespace", podNamespace, "name", podName)
		return nil
	}

	// Find the ReplicaSet owner
	var replicaSetName string
	for _, ownerRef := range pod.OwnerReferences {
		if ownerRef.Kind == "ReplicaSet" {
			replicaSetName = ownerRef.Name
			break
		}
	}

	if replicaSetName == "" {
		log.Info("Pod has no ReplicaSet owner, skipping deployment discovery")
		return nil
	}

	// Get the ReplicaSet
	var replicaSet appsv1.ReplicaSet
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: podNamespace, Name: replicaSetName}, &replicaSet); err != nil {
		log.Error(err, "Failed to get ReplicaSet", "namespace", podNamespace, "name", replicaSetName)
		return nil
	}

	// Find the Deployment owner
	var deploymentName string
	for _, ownerRef := range replicaSet.OwnerReferences {
		if ownerRef.Kind == "Deployment" {
			deploymentName = ownerRef.Name
			break
		}
	}

	if deploymentName == "" {
		log.Info("ReplicaSet has no Deployment owner, skipping deployment discovery")
		return nil
	}

	// Get the Deployment
	var deployment appsv1.Deployment
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: podNamespace, Name: deploymentName}, &deployment); err != nil {
		log.Error(err, "Failed to get Deployment", "namespace", podNamespace, "name", deploymentName)
		return nil
	}

	log.Info("Discovered controller deployment",
		"namespace", podNamespace,
		"name", deployment.Name,
		"uid", deployment.UID,
	)

	// Create and return the owner reference
	// Note: We don't set Controller=true because NodeOverlays are cluster-scoped
	// and the Deployment is namespace-scoped. This is purely for garbage collection.
	return &metav1.OwnerReference{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Name:       deployment.Name,
		UID:        deployment.UID,
	}
}
