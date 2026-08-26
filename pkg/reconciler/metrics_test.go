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
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/nextdoor/veneer/internal/testutil"
	"github.com/nextdoor/veneer/pkg/config"
	"github.com/nextdoor/veneer/pkg/overlay"
	"github.com/nextdoor/veneer/pkg/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

func TestMetricsReconciler_CleanupMissingOverlays(t *testing.T) {
	scheme := runtime.NewScheme()
	gv := schema.GroupVersion{Group: "karpenter.sh", Version: "v1alpha1"}
	scheme.AddKnownTypes(gv, &karpenterv1alpha1.NodeOverlay{}, &karpenterv1alpha1.NodeOverlayList{})
	metav1.AddToGroupVersion(scheme, gv)

	existing := &karpenterv1alpha1.NodeOverlay{ObjectMeta: metav1.ObjectMeta{
		Name: "obsolete-compute",
		Labels: map[string]string{
			overlay.LabelManagedBy:    overlay.LabelManagedByValue,
			overlay.LabelCapacityType: "compute-savings-plan",
		},
	}}
	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(existing).Build()
	reconciler := &MetricsReconciler{Client: client, Logger: logr.Discard()}

	reconciler.cleanupMissingOverlays(context.Background(), nil, map[overlay.CapacityType]bool{
		overlay.CapacityTypeComputeSavingsPlan: true,
	})

	var remaining karpenterv1alpha1.NodeOverlayList
	if err := client.List(context.Background(), &remaining); err != nil {
		t.Fatal(err)
	}
	if len(remaining.Items) != 0 {
		t.Fatalf("expected obsolete observed overlay to be deleted, got %v", remaining.Items)
	}
}

func TestMetricsReconciler_CleanupDisabledOverlayTypes(t *testing.T) {
	scheme := runtime.NewScheme()
	gv := schema.GroupVersion{Group: "karpenter.sh", Version: "v1alpha1"}
	scheme.AddKnownTypes(gv, &karpenterv1alpha1.NodeOverlay{}, &karpenterv1alpha1.NodeOverlayList{})
	metav1.AddToGroupVersion(scheme, gv)

	objects := []runtime.Object{
		&karpenterv1alpha1.NodeOverlay{ObjectMeta: metav1.ObjectMeta{Name: "compute", Labels: map[string]string{
			overlay.LabelManagedBy: overlay.LabelManagedByValue, overlay.LabelCapacityType: "compute-savings-plan",
		}}},
		&karpenterv1alpha1.NodeOverlay{ObjectMeta: metav1.ObjectMeta{Name: "ri", Labels: map[string]string{
			overlay.LabelManagedBy: overlay.LabelManagedByValue, overlay.LabelCapacityType: "reserved-instance",
		}}},
		&karpenterv1alpha1.NodeOverlay{ObjectMeta: metav1.ObjectMeta{Name: "unmanaged-compute", Labels: map[string]string{
			overlay.LabelCapacityType: "compute-savings-plan",
		}}},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objects...).Build()
	reconciler := &MetricsReconciler{
		Config: &config.Config{Overlays: config.OverlayManagementConfig{
			ComputeSavingsPlan:     config.ComputeSavingsPlanOverlayConfig{Enabled: false},
			EC2InstanceSavingsPlan: config.CapacityOverlayConfig{Enabled: true},
			ReservedInstance:       config.CapacityOverlayConfig{Enabled: true},
		}},
		Client: client,
		Logger: logr.Discard(),
	}

	reconciler.cleanupDisabledOverlayTypes(context.Background())

	var remaining karpenterv1alpha1.NodeOverlayList
	if err := client.List(context.Background(), &remaining); err != nil {
		t.Fatal(err)
	}
	if len(remaining.Items) != 2 {
		t.Fatalf("expected 2 overlays to remain, got %d", len(remaining.Items))
	}
	for _, item := range remaining.Items {
		if item.Name == "compute" {
			t.Fatal("managed Compute Savings Plan overlay was not deleted")
		}
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
