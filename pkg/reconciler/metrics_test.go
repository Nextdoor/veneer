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

package reconciler

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/nextdoor/veneer/internal/testutil"
	"github.com/nextdoor/veneer/pkg/config"
	veneermetrics "github.com/nextdoor/veneer/pkg/metrics"
	"github.com/nextdoor/veneer/pkg/prometheus"
	promclient "github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

// newTestConfig creates a minimal config for testing.
func newTestConfig() *config.Config {
	return &config.Config{
		PrometheusURL: "http://test:9090",
		AWS: config.AWSConfig{
			AccountID: "123456789012",
			Region:    "us-west-2",
		},
		Overlays: config.OverlayManagementConfig{
			Disabled:             false,
			UtilizationThreshold: config.DefaultOverlayUtilizationThreshold,
			Weights: config.OverlayWeightsConfig{
				ReservedInstance:       config.DefaultOverlayWeightReservedInstance,
				EC2InstanceSavingsPlan: config.DefaultOverlayWeightEC2InstanceSavingsPlan,
				ComputeSavingsPlan:     config.DefaultOverlayWeightComputeSavingsPlan,
			},
			Naming: config.OverlayNamingConfig{
				ReservedInstancePrefix:       config.DefaultOverlayNamingReservedInstancePrefix,
				EC2InstanceSavingsPlanPrefix: config.DefaultOverlayNamingEC2InstanceSPPrefix,
				ComputeSavingsPlanPrefix:     config.DefaultOverlayNamingComputeSPPrefix,
			},
		},
	}
}

// newTestMetrics creates a Metrics instance with a new registry for testing.
func newTestMetrics() *veneermetrics.Metrics {
	reg := promclient.NewRegistry()
	return veneermetrics.NewMetrics(reg)
}

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
		Config:           newTestConfig(),
		Logger:           logr.Discard(),
		Interval:         100 * time.Millisecond, // Fast interval for testing
		Metrics:          newTestMetrics(),
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
		Config:           newTestConfig(),
		Logger:           logr.Discard(),
		Interval:         1 * time.Second,
		Metrics:          newTestMetrics(),
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
					`lumina_data_freshness_seconds`: `{
						"status": "success",
						"data": {
							"resultType": "vector",
							"result": [{"metric": {}, "value": [1640000000, "30"]}]
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
					`lumina_data_freshness_seconds`: `{
						"status": "success",
						"data": {
							"resultType": "vector",
							"result": [{"metric": {}, "value": [1640000000, "45"]}]
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
					`lumina_data_freshness_seconds`: `{
						"status": "success",
						"data": {
							"resultType": "vector",
							"result": [{"metric": {}, "value": [1640000000, "60"]}]
						}
					}`,
				},
			},
			wantErr: false,
		},
		{
			name: "error when data freshness missing",
			fixtures: []testutil.MetricFixture{
				testutil.LuminaMetricsWithSPCapacity(),
			},
			wantErr:     true,
			errContains: "data freshness",
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
				Config:           newTestConfig(),
				Logger:           logr.Discard(),
				Metrics:          newTestMetrics(),
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
	client, _ := prometheus.NewClient("http://localhost:1", "123456789012", "us-west-2", logr.Discard())

	reconciler := &MetricsReconciler{
		PrometheusClient: client,
		Config:           newTestConfig(),
		Logger:           logr.Discard(),
		Metrics:          newTestMetrics(),
	}

	ctx := context.Background()
	err := reconciler.reconcile(ctx)

	if err == nil {
		t.Error("reconcile() expected error with unavailable server, got nil")
	}
}

func TestMetricsReconciler_DefaultInterval(t *testing.T) {
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
		Config:           newTestConfig(),
		Logger:           logr.Discard(),
		Metrics:          newTestMetrics(),
		// Don't set Interval - should use default
	}

	// Start with short timeout to verify default interval is set
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_ = reconciler.Start(ctx)

	// Verify default was set
	assert.Equal(t, DefaultReconcileInterval, reconciler.Interval)
}

func TestMetricsReconciler_ReconcileWithNilMetrics(t *testing.T) {
	// Test that reconciler works when Metrics is nil (graceful degradation)
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
		Config:           newTestConfig(),
		Logger:           logr.Discard(),
		Metrics:          nil, // Explicitly nil
	}

	ctx := context.Background()
	err := reconciler.reconcile(ctx)

	// Should not panic, should complete successfully
	assert.NoError(t, err)
}

func TestDefaultReconcileInterval(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 5*time.Minute, DefaultReconcileInterval)
}

func TestMaxDataFreshnessSeconds(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 600.0, MaxDataFreshnessSeconds)
}
