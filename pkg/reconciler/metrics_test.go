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
	"github.com/nextdoor/veneer/pkg/prometheus"
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
							"result": [{"metric": {"account_id": "123456789012", "data_type": "savings_plans"}, "value": [1640000000, "30"]}]
						}
					}`,
					`lumina_data_freshness_seconds{account_id="123456789012", data_type="reserved_instances"}`: `{
						"status": "success",
						"data": {
							"resultType": "vector",
							"result": [{"metric": {"account_id": "123456789012", "data_type": "reserved_instances"}, "value": [1640000000, "30"]}]
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
							"result": [{"metric": {"account_id": "123456789012", "data_type": "savings_plans"}, "value": [1640000000, "45"]}]
						}
					}`,
					`lumina_data_freshness_seconds{account_id="123456789012", data_type="reserved_instances"}`: `{
						"status": "success",
						"data": {
							"resultType": "vector",
							"result": [{"metric": {"account_id": "123456789012", "data_type": "reserved_instances"}, "value": [1640000000, "45"]}]
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
							"result": [{"metric": {"account_id": "123456789012", "data_type": "savings_plans"}, "value": [1640000000, "60"]}]
						}
					}`,
					`lumina_data_freshness_seconds{account_id="123456789012", data_type="reserved_instances"}`: `{
						"status": "success",
						"data": {
							"resultType": "vector",
							"result": [{"metric": {"account_id": "123456789012", "data_type": "reserved_instances"}, "value": [1640000000, "60"]}]
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

func TestMetricsReconciler_DefaultInterval(t *testing.T) {
	server := testutil.NewMockPrometheusServer()
	defer server.Close()

	server.SetMetrics(testutil.LuminaMetricsWithSPCapacity())
	server.SetMetrics(testutil.MetricFixture{
		`lumina_data_freshness_seconds{account_id="123456789012", data_type="savings_plans"}`: `{
			"status": "success",
			"data": {
				"resultType": "vector",
				"result": [{"metric": {"account_id": "123456789012", "data_type": "savings_plans"}, "value": [1640000000, "30"]}]
			}
		}`,
		`lumina_data_freshness_seconds{account_id="123456789012", data_type="reserved_instances"}`: `{
			"status": "success",
			"data": {
				"resultType": "vector",
				"result": [{"metric": {"account_id": "123456789012", "data_type": "reserved_instances"}, "value": [1640000000, "30"]}]
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
