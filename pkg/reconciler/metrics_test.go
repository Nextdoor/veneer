// Copyright 2025 Nextdoor, Inc.
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

package reconciler

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/nextdoor/veneer/internal/testutil"
	"github.com/nextdoor/veneer/pkg/config"
	veneermetrics "github.com/nextdoor/veneer/pkg/metrics"
	"github.com/nextdoor/veneer/pkg/overlay"
	"github.com/nextdoor/veneer/pkg/preference"
	"github.com/nextdoor/veneer/pkg/prometheus"
	promclient "github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	karpenterv1alpha1 "sigs.k8s.io/karpenter/pkg/apis/v1alpha1"
)

func TestMetricsReconciler_Start(t *testing.T) {
	server := testutil.NewMockPrometheusServer()
	defer server.Close()

	// Set up metrics with SP capacity and data freshness
	server.SetMetrics(testutil.LuminaMetricsWithSPCapacity())
	server.SetMetrics(testutil.MetricFixture{
		`lumina_data_freshness_seconds`: `{
			"status": "success",
			"data": {
				"resultType": "vector",
				"result": [{"metric": {}, "value": [1640000000, "30"]}]
			}
		}`,
	})

	client, err := prometheus.NewClient(server.URL, "123456789012", "us-west-2", logr.Discard())
	if err != nil {
		t.Fatalf("Failed to create Prometheus client: %v", err)
	}

	reconciler := &MetricsReconciler{
		PrometheusClient: client,
		Logger:           logr.Discard(),
		Interval:         100 * time.Millisecond, // Fast interval for testing
	}

	// Start reconciler in background
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	err = reconciler.Start(ctx)
	if err != nil {
		t.Errorf("Start() returned unexpected error: %v", err)
	}

	// Start should run at least twice (once immediately, once on ticker)
	// If we got here without error, the reconciler ran successfully
}

func TestMetricsReconciler_StartWithCancel(t *testing.T) {
	server := testutil.NewMockPrometheusServer()
	defer server.Close()

	server.SetMetrics(testutil.LuminaMetricsWithSPCapacity())
	server.SetMetrics(testutil.MetricFixture{
		`lumina_data_freshness_seconds`: `{
			"status": "success",
			"data": {
				"resultType": "vector",
				"result": [{"metric": {}, "value": [1640000000, "30"]}]
			}
		}`,
	})

	client, _ := prometheus.NewClient(server.URL, "123456789012", "us-west-2", logr.Discard())

	reconciler := &MetricsReconciler{
		PrometheusClient: client,
		Logger:           logr.Discard(),
		Interval:         1 * time.Second,
	}

	// Start and immediately cancel
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := reconciler.Start(ctx)
	if err != nil {
		t.Errorf("Start() with cancelled context returned unexpected error: %v", err)
	}
}

func TestMetricsReconciler_Reconcile(t *testing.T) {
	tests := []struct {
		name        string
		fixtures    []testutil.MetricFixture
		wantErr     bool
		errContains string
	}{
		{
			name: "successful reconcile with SP capacity",
			fixtures: []testutil.MetricFixture{
				testutil.LuminaMetricsWithSPCapacity(),
				{
					`lumina_data_freshness_seconds{account_id="123456789012", data_type="savings_plans"}`: `{
						"status": "success",
						"data": {
							"resultType": "vector",
							"result": [{
								"metric": {"account_id": "123456789012", "data_type": "savings_plans"},
								"value": [1640000000, "30"]
							}]
						}
					}`,
					`lumina_data_freshness_seconds{account_id="123456789012", data_type="reserved_instances"}`: `{
						"status": "success",
						"data": {
							"resultType": "vector",
							"result": [{
								"metric": {"account_id": "123456789012", "data_type": "reserved_instances"},
								"value": [1640000000, "30"]
							}]
						}
					}`,
				},
			},
			wantErr: false,
		},
		{
			name: "successful reconcile with no capacity",
			fixtures: []testutil.MetricFixture{
				testutil.LuminaMetricsWithNoCapacity(),
				{
					`lumina_data_freshness_seconds{account_id="123456789012", data_type="savings_plans"}`: `{
						"status": "success",
						"data": {
							"resultType": "vector",
							"result": [{
								"metric": {"account_id": "123456789012", "data_type": "savings_plans"},
								"value": [1640000000, "45"]
							}]
						}
					}`,
					`lumina_data_freshness_seconds{account_id="123456789012", data_type="reserved_instances"}`: `{
						"status": "success",
						"data": {
							"resultType": "vector",
							"result": [{
								"metric": {"account_id": "123456789012", "data_type": "reserved_instances"},
								"value": [1640000000, "45"]
							}]
						}
					}`,
				},
			},
			wantErr: false,
		},
		{
			name: "successful reconcile with empty metrics",
			fixtures: []testutil.MetricFixture{
				testutil.LuminaMetricsEmpty(),
				{
					`lumina_data_freshness_seconds{account_id="123456789012", data_type="savings_plans"}`: `{
						"status": "success",
						"data": {
							"resultType": "vector",
							"result": [{
								"metric": {"account_id": "123456789012", "data_type": "savings_plans"},
								"value": [1640000000, "60"]
							}]
						}
					}`,
					`lumina_data_freshness_seconds{account_id="123456789012", data_type="reserved_instances"}`: `{
						"status": "success",
						"data": {
							"resultType": "vector",
							"result": [{
								"metric": {"account_id": "123456789012", "data_type": "reserved_instances"},
								"value": [1640000000, "60"]
							}]
						}
					}`,
				},
			},
			wantErr: false,
		},
		{
			// When freshness data is missing, we now gracefully continue (logging error) instead of failing
			name: "graceful continue when data freshness missing",
			fixtures: []testutil.MetricFixture{
				testutil.LuminaMetricsWithSPCapacity(),
			},
			wantErr: false, // Changed: no longer returns error, just logs and continues
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := testutil.NewMockPrometheusServer()
			defer server.Close()

			for _, fixture := range tt.fixtures {
				server.SetMetrics(fixture)
			}

			client, _ := prometheus.NewClient(server.URL, "123456789012", "us-west-2", logr.Discard())

			reconciler := &MetricsReconciler{
				PrometheusClient: client,
				Logger:           logr.Discard(),
			}

			ctx := context.Background()
			err := reconciler.reconcile(ctx)

			if (err != nil) != tt.wantErr {
				t.Errorf("reconcile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errContains != "" {
				if err == nil || len(err.Error()) == 0 {
					t.Errorf("expected error containing %q, got nil", tt.errContains)
				}
			}
		})
	}
}

func TestMetricsReconciler_ReconcileWithServerError(t *testing.T) {
	// Use unavailable server to trigger connection errors
	// The reconciler now gracefully handles errors and continues instead of failing
	client, _ := prometheus.NewClient("http://localhost:1", "123456789012", "us-west-2", logr.Discard())

	reconciler := &MetricsReconciler{
		PrometheusClient: client,
		Logger:           logr.Discard(),
	}

	ctx := context.Background()
	err := reconciler.reconcile(ctx)

	// With the new design, reconcile() logs errors but doesn't return them
	// This allows partial reconciliation when some data sources are unavailable
	if err != nil {
		t.Errorf("reconcile() unexpected error: %v (should gracefully handle server errors)", err)
	}
}

func TestMetricsReconciler_AnalyzeComputeSavingsPlansSeedsExistingOverlay(t *testing.T) {
	tests := []struct {
		name      string
		managed   bool
		wantExist bool
	}{
		{name: "managed overlay is adopted", managed: true, wantExist: true},
		{name: "unmanaged overlay still waits", managed: false, wantExist: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := testutil.NewMockPrometheusServer()
			defer server.Close()
			server.SetMetrics(
				testutil.LuminaMetricsWithSPCapacity(),
				testutil.LuminaMetricsWithSPUtilization(),
			)
			promClient, err := prometheus.NewClient(
				server.URL,
				"123456789012",
				"us-west-2",
				logr.Discard(),
			)
			if err != nil {
				t.Fatal(err)
			}

			cfg := metricsTestConfig()
			now := time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC)
			engine := overlay.NewDecisionEngineWithClock(cfg, func() time.Time { return now })
			kubeClient := fake.NewClientBuilder().
				WithScheme(metricsTestScheme(t)).
				WithRuntimeObjects(costOverlay(
					"cost-aware-compute-sp-global",
					"compute-savings-plan",
					tt.managed,
				)).
				Build()
			reconciler := &MetricsReconciler{
				PrometheusClient: promClient,
				Config:           cfg,
				DecisionEngine:   engine,
				Client:           kubeClient,
				Logger:           logr.Discard(),
			}

			decisions, err := reconciler.analyzeComputeSavingsPlans(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(decisions) != 1 {
				t.Fatalf("decisions = %d, want 1", len(decisions))
			}
			if decisions[0].ShouldExist != tt.wantExist {
				t.Fatalf("decision = %+v, want ShouldExist=%v", decisions[0], tt.wantExist)
			}
			if !tt.wantExist && !strings.Contains(decisions[0].Reason, "waiting") {
				t.Fatalf("unmanaged overlay reason = %q, want dwell wait", decisions[0].Reason)
			}
		})
	}
}

func TestMetricsReconciler_ReconcileResetsComputeEligibility(t *testing.T) {
	const promError = `{
		"status": "error",
		"errorType": "execution",
		"error": "fixture failure"
	}`
	tests := []struct {
		name     string
		fixtures []testutil.MetricFixture
	}{
		{
			name: "stale freshness",
			fixtures: []testutil.MetricFixture{{
				`lumina_data_freshness_seconds{account_id="123456789012", data_type="savings_plans"}`: prometheusVector("4000"),
			}},
		},
		{
			name: "freshness query error",
			fixtures: []testutil.MetricFixture{{
				`lumina_data_freshness_seconds{account_id="123456789012", data_type="savings_plans"}`: promError,
			}},
		},
		{
			name: "analysis query error",
			fixtures: []testutil.MetricFixture{
				freshnessFixture(30),
				{
					`savings_plan_utilization_percent{account_id="123456789012", type="compute"}`: promError,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := metricsTestConfig()
			now := time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC)
			engine := overlay.NewDecisionEngineWithClock(cfg, func() time.Time { return now })
			eligible := overlay.AggregatedSavingsPlan{
				UtilizationPercent:     80,
				TotalRemainingCapacity: 100,
			}
			if first := engine.AnalyzeComputeSavingsPlan(eligible); first.ShouldExist {
				t.Fatalf("first observation should start dwell: %+v", first)
			}
			now = now.Add(cfg.Overlays.ComputeSavingsPlan.MinBelowThresholdDuration)

			server := testutil.NewMockPrometheusServer()
			defer server.Close()
			server.SetMetrics(tt.fixtures...)
			promClient, err := prometheus.NewClient(
				server.URL,
				"123456789012",
				"us-west-2",
				logr.Discard(),
			)
			if err != nil {
				t.Fatal(err)
			}
			reconciler := &MetricsReconciler{
				PrometheusClient: promClient,
				Config:           cfg,
				DecisionEngine:   engine,
				Logger:           logr.Discard(),
			}
			if err := reconciler.reconcile(context.Background()); err != nil {
				t.Fatal(err)
			}

			afterReset := engine.AnalyzeComputeSavingsPlan(eligible)
			if afterReset.ShouldExist || !strings.Contains(afterReset.Reason, "waiting") {
				t.Fatalf("telemetry interruption did not reset dwell: %+v", afterReset)
			}
		})
	}
}

func TestMetricsReconciler_ReconcileDeletesComputeOverlayWithNoCapacityData(t *testing.T) {
	server := testutil.NewMockPrometheusServer()
	defer server.Close()
	server.SetMetrics(testutil.LuminaMetricsEmpty(), freshnessFixture(30))
	promClient, err := prometheus.NewClient(
		server.URL,
		"123456789012",
		"us-west-2",
		logr.Discard(),
	)
	if err != nil {
		t.Fatal(err)
	}

	cfg := metricsTestConfig()
	kubeClient := fake.NewClientBuilder().
		WithScheme(metricsTestScheme(t)).
		WithRuntimeObjects(costOverlay(
			"cost-aware-compute-sp-global",
			"compute-savings-plan",
			true,
		)).
		Build()
	reconciler := &MetricsReconciler{
		PrometheusClient: promClient,
		Config:           cfg,
		DecisionEngine:   overlay.NewDecisionEngine(cfg),
		Generator:        overlay.NewGenerator(),
		Client:           kubeClient,
		Logger:           logr.Discard(),
	}
	if err := reconciler.reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}

	err = kubeClient.Get(
		context.Background(),
		ctrlclient.ObjectKey{Name: "cost-aware-compute-sp-global"},
		&karpenterv1alpha1.NodeOverlay{},
	)
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected no-data Compute overlay deletion, got %v", err)
	}
}

func TestMetricsReconciler_CleanupMissingOverlays(t *testing.T) {
	objects := []runtime.Object{
		costOverlay("obsolete-compute", "compute-savings-plan", true),
		costOverlay("desired-compute", "compute-savings-plan", true),
		costOverlay("unobserved-ec2", "ec2-instance-savings-plan", true),
		costOverlay("unknown-capacity-type", "future-type", true),
		costOverlay("unmanaged-compute", "compute-savings-plan", false),
		&karpenterv1alpha1.NodeOverlay{ObjectMeta: metav1.ObjectMeta{
			Name: "pref-general-1",
			Labels: map[string]string{
				overlay.LabelManagedBy:           overlay.LabelManagedByValue,
				overlay.LabelCapacityType:        "compute-savings-plan",
				preference.LabelPreferenceType:   preference.LabelPreferenceTypeValue,
				preference.LabelSourceNodePool:   "general",
				preference.LabelPreferenceNumber: "1",
			},
		}},
	}
	kubeClient := fake.NewClientBuilder().
		WithScheme(metricsTestScheme(t)).
		WithRuntimeObjects(objects...).
		Build()
	registry := promclient.NewRegistry()
	controllerMetrics := veneermetrics.NewMetrics(registry)
	reconciler := &MetricsReconciler{
		Client:  kubeClient,
		Logger:  logr.Discard(),
		Metrics: controllerMetrics,
	}

	reconciler.cleanupMissingOverlays(
		context.Background(),
		[]overlay.Decision{{Name: "desired-compute"}},
		map[overlay.CapacityType]bool{overlay.CapacityTypeComputeSavingsPlan: true},
	)

	var remaining karpenterv1alpha1.NodeOverlayList
	if err := kubeClient.List(context.Background(), &remaining); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"desired-compute":       true,
		"unobserved-ec2":        true,
		"unknown-capacity-type": true,
		"unmanaged-compute":     true,
		"pref-general-1":        true,
	}
	if len(remaining.Items) != len(want) {
		t.Fatalf("remaining overlays = %v, want names %v", overlayNames(remaining.Items), want)
	}
	for _, item := range remaining.Items {
		if !want[item.Name] {
			t.Fatalf("unexpected remaining overlay %q", item.Name)
		}
	}
	gotDeletes := promtest.ToFloat64(controllerMetrics.OverlayOperationsTotal.WithLabelValues(
		veneermetrics.OperationDelete.String(),
		veneermetrics.CapacityTypeComputeSP.String(),
	))
	if gotDeletes != 1 {
		t.Fatalf("compute delete operations = %v, want 1", gotDeletes)
	}
}

func TestMetricsReconciler_CleanupDisabledOverlayTypes(t *testing.T) {
	objects := []runtime.Object{
		costOverlay("compute", "compute-savings-plan", true),
		costOverlay("ri", "reserved-instance", true),
		costOverlay("unmanaged-compute", "compute-savings-plan", false),
	}
	kubeClient := fake.NewClientBuilder().
		WithScheme(metricsTestScheme(t)).
		WithRuntimeObjects(objects...).
		Build()
	registry := promclient.NewRegistry()
	controllerMetrics := veneermetrics.NewMetrics(registry)
	reconciler := &MetricsReconciler{
		Config: &config.Config{Overlays: config.OverlayManagementConfig{
			ComputeSavingsPlan:     config.ComputeSavingsPlanOverlayConfig{Enabled: false},
			EC2InstanceSavingsPlan: config.CapacityOverlayConfig{Enabled: true},
			ReservedInstance:       config.CapacityOverlayConfig{Enabled: true},
		}},
		Client:  kubeClient,
		Logger:  logr.Discard(),
		Metrics: controllerMetrics,
	}

	reconciler.cleanupDisabledOverlayTypes(context.Background())

	var remaining karpenterv1alpha1.NodeOverlayList
	if err := kubeClient.List(context.Background(), &remaining); err != nil {
		t.Fatal(err)
	}
	if got := overlayNames(remaining.Items); len(got) != 2 || got["compute"] {
		t.Fatalf("remaining overlays = %v, want enabled and unmanaged overlays only", got)
	}
	gotDeletes := promtest.ToFloat64(controllerMetrics.OverlayOperationsTotal.WithLabelValues(
		veneermetrics.OperationDelete.String(),
		veneermetrics.CapacityTypeComputeSP.String(),
	))
	if gotDeletes != 1 {
		t.Fatalf("compute delete operations = %v, want 1", gotDeletes)
	}
}

func metricsTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	gv := schema.GroupVersion{Group: "karpenter.sh", Version: "v1alpha1"}
	scheme.AddKnownTypes(gv, &karpenterv1alpha1.NodeOverlay{}, &karpenterv1alpha1.NodeOverlayList{})
	metav1.AddToGroupVersion(scheme, gv)
	return scheme
}

func metricsTestConfig() *config.Config {
	return &config.Config{Overlays: config.OverlayManagementConfig{
		UtilizationThreshold: 95,
		ComputeSavingsPlan: config.ComputeSavingsPlanOverlayConfig{
			Enabled:                     true,
			PriceAdjustment:             "-50%",
			MinRemainingCapacityDollars: 10,
			MinBelowThresholdDuration:   15 * time.Minute,
		},
		EC2InstanceSavingsPlan: config.CapacityOverlayConfig{
			Enabled:         false,
			PriceAdjustment: "-90%",
		},
		ReservedInstance: config.CapacityOverlayConfig{
			Enabled:         false,
			PriceAdjustment: "-90%",
		},
		Weights: config.OverlayWeightsConfig{
			ComputeSavingsPlan:     10,
			EC2InstanceSavingsPlan: 20,
			ReservedInstance:       30,
		},
	}}
}

func costOverlay(name, capacityType string, managed bool) *karpenterv1alpha1.NodeOverlay {
	labels := map[string]string{overlay.LabelCapacityType: capacityType}
	if managed {
		labels[overlay.LabelManagedBy] = overlay.LabelManagedByValue
	}
	return &karpenterv1alpha1.NodeOverlay{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels}}
}

func overlayNames(items []karpenterv1alpha1.NodeOverlay) map[string]bool {
	names := make(map[string]bool, len(items))
	for _, item := range items {
		names[item.Name] = true
	}
	return names
}

func freshnessFixture(age float64) testutil.MetricFixture {
	value := fmt.Sprintf("%g", age)
	return testutil.MetricFixture{
		`lumina_data_freshness_seconds{account_id="123456789012", data_type="savings_plans"}`:      prometheusVector(value),
		`lumina_data_freshness_seconds{account_id="123456789012", data_type="reserved_instances"}`: prometheusVector(value),
	}
}

func prometheusVector(value string) string {
	return fmt.Sprintf(
		`{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1640000000,%q]}]}}`,
		value,
	)
}

func TestMetricsReconciler_DefaultInterval(t *testing.T) {
	server := testutil.NewMockPrometheusServer()
	defer server.Close()

	server.SetMetrics(testutil.LuminaMetricsWithSPCapacity())
	server.SetMetrics(testutil.MetricFixture{
		`lumina_data_freshness_seconds{account_id="123456789012", data_type="savings_plans"}`: `{
			"status": "success",
			"data": {
				"resultType": "vector",
				"result": [{
					"metric": {"account_id": "123456789012", "data_type": "savings_plans"},
					"value": [1640000000, "30"]
				}]
			}
		}`,
		`lumina_data_freshness_seconds{account_id="123456789012", data_type="reserved_instances"}`: `{
			"status": "success",
			"data": {
				"resultType": "vector",
				"result": [{
					"metric": {"account_id": "123456789012", "data_type": "reserved_instances"},
					"value": [1640000000, "30"]
				}]
			}
		}`,
	})

	client, _ := prometheus.NewClient(server.URL, "123456789012", "us-west-2", logr.Discard())

	reconciler := &MetricsReconciler{
		PrometheusClient: client,
		Logger:           logr.Discard(),
		// Don't set Interval - should use default
	}

	// Start with short timeout to verify default interval is set
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_ = reconciler.Start(ctx)

	// Verify default was set
	if reconciler.Interval != 5*time.Minute {
		t.Errorf("Expected default interval 5m, got %v", reconciler.Interval)
	}
}
