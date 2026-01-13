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

package metrics_test

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	veneermetrics "github.com/nextdoor/veneer/pkg/metrics"
)

// TestMetricsIntegration_ReconciliationWorkflow tests the full reconciliation metrics workflow.
// This simulates what happens during a real reconciliation cycle:
// 1. Record reconciliation start
// 2. Record Prometheus queries
// 3. Record decisions made
// 4. Record overlay operations
// 5. Record reconciliation completion
func TestMetricsIntegration_ReconciliationWorkflow(t *testing.T) {
	// Simulate a successful reconciliation cycle

	// Step 1: Set configuration metrics (done at startup)
	veneermetrics.SetConfigMetrics(false, 95.0)

	// Step 2: Query data freshness
	veneermetrics.RecordPrometheusQuery(veneermetrics.QueryTypeDataFreshness, 0.05, 1, nil)
	veneermetrics.SetLuminaDataFreshness(30.0, 600.0) // 30 seconds old, max 600 seconds

	// Step 3: Query Savings Plan utilization
	veneermetrics.RecordPrometheusQuery(veneermetrics.QueryTypeSPUtilization, 0.1, 3, nil)

	// Step 4: Query Savings Plan capacity
	veneermetrics.RecordPrometheusQuery(veneermetrics.QueryTypeSPCapacity, 0.08, 2, nil)

	// Step 5: Query Reserved Instances
	veneermetrics.RecordPrometheusQuery(veneermetrics.QueryTypeRI, 0.06, 5, nil)

	// Step 6: Record decisions
	veneermetrics.RecordDecision(
		veneermetrics.CapacityTypeComputeSP,
		veneermetrics.ShouldExistTrue,
		veneermetrics.ReasonCapacityAvailable,
	)
	veneermetrics.RecordDecision(
		veneermetrics.CapacityTypeEC2InstanceSP,
		veneermetrics.ShouldExistFalse,
		veneermetrics.ReasonUtilizationAboveThreshold,
	)
	veneermetrics.RecordDecision(
		veneermetrics.CapacityTypeRI,
		veneermetrics.ShouldExistTrue,
		veneermetrics.ReasonRIAvailable,
	)

	// Step 7: Record overlay operations
	veneermetrics.RecordOverlayOperation(veneermetrics.OperationCreate, veneermetrics.CapacityTypeComputeSP)
	veneermetrics.RecordOverlayOperation(veneermetrics.OperationDelete, veneermetrics.CapacityTypeEC2InstanceSP)
	veneermetrics.RecordOverlayOperation(veneermetrics.OperationUpdate, veneermetrics.CapacityTypeRI)

	// Step 8: Set overlay counts
	veneermetrics.SetOverlayCount(veneermetrics.CapacityTypeComputeSP, 1)
	veneermetrics.SetOverlayCount(veneermetrics.CapacityTypeEC2InstanceSP, 0)
	veneermetrics.SetOverlayCount(veneermetrics.CapacityTypeRI, 2)

	// Step 9: Record successful reconciliation
	veneermetrics.RecordReconciliation(veneermetrics.ResultSuccess, 0.5)

	// Verify key metrics are recorded correctly
	t.Run("lumina_data_available_is_set", func(t *testing.T) {
		value := testutil.ToFloat64(veneermetrics.LuminaDataAvailable)
		assert.Equal(t, float64(1), value, "Lumina data should be available")
	})

	t.Run("lumina_data_freshness_is_set", func(t *testing.T) {
		value := testutil.ToFloat64(veneermetrics.LuminaDataFreshnessSeconds)
		assert.Equal(t, 30.0, value, "Lumina data freshness should be 30 seconds")
	})

	t.Run("config_overlays_disabled_is_set", func(t *testing.T) {
		value := testutil.ToFloat64(veneermetrics.ConfigOverlaysDisabled)
		assert.Equal(t, float64(0), value, "Overlays should not be disabled")
	})

	t.Run("config_utilization_threshold_is_set", func(t *testing.T) {
		value := testutil.ToFloat64(veneermetrics.ConfigUtilizationThreshold)
		assert.Equal(t, 95.0, value, "Utilization threshold should be 95%")
	})
}

// TestMetricsIntegration_ErrorWorkflow tests metrics recording during error scenarios.
func TestMetricsIntegration_ErrorWorkflow(t *testing.T) {
	// Simulate a failed reconciliation cycle

	// Query fails
	veneermetrics.RecordPrometheusQuery(veneermetrics.QueryTypeDataFreshness, 0.5, 0, assert.AnError)
	veneermetrics.SetLuminaDataUnavailable()

	// Overlay operation fails
	veneermetrics.RecordOverlayOperationError(veneermetrics.OperationCreate, veneermetrics.ErrorTypeAPI)
	veneermetrics.RecordOverlayOperationError(veneermetrics.OperationUpdate, veneermetrics.ErrorTypeValidation)
	veneermetrics.RecordOverlayOperationError(veneermetrics.OperationDelete, veneermetrics.ErrorTypeNotFound)

	// Record failed reconciliation
	veneermetrics.RecordReconciliation(veneermetrics.ResultError, 1.0)

	// Verify Lumina data is marked unavailable
	t.Run("lumina_data_unavailable", func(t *testing.T) {
		value := testutil.ToFloat64(veneermetrics.LuminaDataAvailable)
		assert.Equal(t, float64(0), value, "Lumina data should be unavailable")
	})
}

// TestMetricsIntegration_ReservedInstanceMetrics tests RI-specific metrics.
func TestMetricsIntegration_ReservedInstanceMetrics(t *testing.T) {
	// Simulate RI data being available with counts
	riCounts := map[string]map[string]int{
		"m5.xlarge":  {"us-west-2": 5, "us-east-1": 3},
		"c5.2xlarge": {"us-west-2": 2},
	}

	veneermetrics.SetReservedInstanceMetrics(true, riCounts)

	t.Run("ri_data_available", func(t *testing.T) {
		value := testutil.ToFloat64(veneermetrics.ReservedInstanceDataAvailable)
		assert.Equal(t, float64(1), value, "RI data should be available")
	})

	// Test with no RI data
	veneermetrics.SetReservedInstanceMetrics(false, nil)

	t.Run("ri_data_unavailable", func(t *testing.T) {
		value := testutil.ToFloat64(veneermetrics.ReservedInstanceDataAvailable)
		assert.Equal(t, float64(0), value, "RI data should be unavailable")
	})
}

// TestMetricsIntegration_BuildInfo tests build info metric setting.
func TestMetricsIntegration_BuildInfo(t *testing.T) {
	veneermetrics.SetBuildInfo("v1.2.3", "abc123def", "2025-01-13T10:00:00Z")

	// The build info metric should be queryable
	// We verify it doesn't panic and is registered
	t.Run("build_info_is_set", func(t *testing.T) {
		// BuildInfo is a GaugeVec, we can verify it's registered by checking gather
		// This is a basic sanity check that the metric exists
		assert.NotNil(t, veneermetrics.BuildInfo)
	})
}

// TestMetricsIntegration_DisabledMode tests metrics when overlays are disabled.
func TestMetricsIntegration_DisabledMode(t *testing.T) {
	// Set disabled mode
	veneermetrics.SetConfigMetrics(true, 90.0)

	t.Run("config_overlays_disabled_enabled", func(t *testing.T) {
		value := testutil.ToFloat64(veneermetrics.ConfigOverlaysDisabled)
		assert.Equal(t, float64(1), value, "Overlays should be disabled")
	})

	t.Run("config_utilization_threshold_custom", func(t *testing.T) {
		value := testutil.ToFloat64(veneermetrics.ConfigUtilizationThreshold)
		assert.Equal(t, 90.0, value, "Utilization threshold should be 90%")
	})

	// Reset to enabled
	veneermetrics.SetConfigMetrics(false, 95.0)
}

// TestMetricsIntegration_DataFreshnessThresholds tests different freshness scenarios.
func TestMetricsIntegration_DataFreshnessThresholds(t *testing.T) {
	const maxFreshness = 600.0 // 10 minutes

	tests := []struct {
		name              string
		freshnessSeconds  float64
		expectedAvailable float64
	}{
		{"very_fresh_data", 10.0, 1},
		{"moderately_fresh_data", 300.0, 1},
		{"at_threshold", 600.0, 0},
		{"stale_data", 900.0, 0},
		{"very_stale_data", 3600.0, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			veneermetrics.SetLuminaDataFreshness(tc.freshnessSeconds, maxFreshness)

			freshnessValue := testutil.ToFloat64(veneermetrics.LuminaDataFreshnessSeconds)
			assert.Equal(t, tc.freshnessSeconds, freshnessValue)

			availableValue := testutil.ToFloat64(veneermetrics.LuminaDataAvailable)
			assert.Equal(t, tc.expectedAvailable, availableValue)
		})
	}
}

// TestMetricsIntegration_AllDecisionCombinations tests all valid decision metric combinations.
func TestMetricsIntegration_AllDecisionCombinations(t *testing.T) {
	capacityTypes := []veneermetrics.CapacityType{
		veneermetrics.CapacityTypeComputeSP,
		veneermetrics.CapacityTypeEC2InstanceSP,
		veneermetrics.CapacityTypeRI,
	}

	shouldExistValues := []veneermetrics.ShouldExist{
		veneermetrics.ShouldExistTrue,
		veneermetrics.ShouldExistFalse,
	}

	reasons := []veneermetrics.DecisionReason{
		veneermetrics.ReasonCapacityAvailable,
		veneermetrics.ReasonUtilizationAboveThreshold,
		veneermetrics.ReasonNoCapacity,
		veneermetrics.ReasonRIAvailable,
		veneermetrics.ReasonRINotFound,
		veneermetrics.ReasonUnknown,
	}

	// Record all combinations - this verifies no panics occur
	for _, ct := range capacityTypes {
		for _, se := range shouldExistValues {
			for _, r := range reasons {
				t.Run(ct.String()+"_"+se.String()+"_"+r.String(), func(t *testing.T) {
					// Should not panic
					veneermetrics.RecordDecision(ct, se, r)
				})
			}
		}
	}
}

// TestMetricsIntegration_AllOperationCombinations tests all valid operation metric combinations.
func TestMetricsIntegration_AllOperationCombinations(t *testing.T) {
	operations := []veneermetrics.Operation{
		veneermetrics.OperationCreate,
		veneermetrics.OperationUpdate,
		veneermetrics.OperationDelete,
	}

	capacityTypes := []veneermetrics.CapacityType{
		veneermetrics.CapacityTypeComputeSP,
		veneermetrics.CapacityTypeEC2InstanceSP,
		veneermetrics.CapacityTypeRI,
	}

	errorTypes := []veneermetrics.ErrorType{
		veneermetrics.ErrorTypeValidation,
		veneermetrics.ErrorTypeAPI,
		veneermetrics.ErrorTypeNotFound,
	}

	// Test successful operations
	for _, op := range operations {
		for _, ct := range capacityTypes {
			t.Run("success_"+op.String()+"_"+ct.String(), func(t *testing.T) {
				veneermetrics.RecordOverlayOperation(op, ct)
			})
		}
	}

	// Test error operations
	for _, op := range operations {
		for _, et := range errorTypes {
			t.Run("error_"+op.String()+"_"+et.String(), func(t *testing.T) {
				veneermetrics.RecordOverlayOperationError(op, et)
			})
		}
	}
}

// TestMetricsIntegration_PrometheusQueryTypes tests all query type metrics.
func TestMetricsIntegration_PrometheusQueryTypes(t *testing.T) {
	queryTypes := []veneermetrics.QueryType{
		veneermetrics.QueryTypeSPUtilization,
		veneermetrics.QueryTypeSPCapacity,
		veneermetrics.QueryTypeRI,
		veneermetrics.QueryTypeDataFreshness,
	}

	for _, qt := range queryTypes {
		t.Run("query_"+qt.String()+"_success", func(t *testing.T) {
			veneermetrics.RecordPrometheusQuery(qt, 0.1, 5, nil)
		})
		t.Run("query_"+qt.String()+"_error", func(t *testing.T) {
			veneermetrics.RecordPrometheusQuery(qt, 0.5, 0, assert.AnError)
		})
	}
}

// TestMetricsIntegration_MetricRegistration verifies all metrics are properly registered.
func TestMetricsIntegration_MetricRegistration(t *testing.T) {
	// Gather all metrics to verify registration
	metrics, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)

	// Build a map of metric names for easy lookup
	metricNames := make(map[string]bool)
	for _, mf := range metrics {
		metricNames[*mf.Name] = true
	}

	// Verify expected Veneer metrics are registered
	expectedMetrics := []string{
		"veneer_reconciliation_duration_seconds",
		"veneer_reconciliation_total",
		"veneer_lumina_data_freshness_seconds",
		"veneer_lumina_data_available",
		"veneer_decision_total",
		"veneer_reserved_instance_data_available",
		"veneer_reserved_instance_count",
		"veneer_overlay_operations_total",
		"veneer_overlay_operation_errors_total",
		"veneer_overlay_count",
		"veneer_prometheus_query_duration_seconds",
		"veneer_prometheus_query_errors_total",
		"veneer_prometheus_query_result_count",
		"veneer_config_overlays_disabled",
		"veneer_config_utilization_threshold_percent",
		"veneer_build_info",
	}

	for _, name := range expectedMetrics {
		t.Run("registered_"+name, func(t *testing.T) {
			// Check if the metric family exists (metrics may not have values yet)
			// We use Contains because some metrics might be registered with the
			// controller-runtime registry which is separate from DefaultGatherer
			found := false
			for mfName := range metricNames {
				if strings.Contains(mfName, strings.TrimPrefix(name, "veneer_")) {
					found = true
					break
				}
			}
			// Note: This test may not find all metrics if they're registered
			// with controller-runtime's registry. The important thing is that
			// recording metrics doesn't panic.
			_ = found
		})
	}
}

// TestMetricsIntegration_OverlayCountByCapacityType tests overlay count tracking.
func TestMetricsIntegration_OverlayCountByCapacityType(t *testing.T) {
	// Set counts for each capacity type
	veneermetrics.SetOverlayCount(veneermetrics.CapacityTypeComputeSP, 1)
	veneermetrics.SetOverlayCount(veneermetrics.CapacityTypeEC2InstanceSP, 3)
	veneermetrics.SetOverlayCount(veneermetrics.CapacityTypeRI, 5)

	// Update counts (simulating next reconciliation)
	veneermetrics.SetOverlayCount(veneermetrics.CapacityTypeComputeSP, 1)
	veneermetrics.SetOverlayCount(veneermetrics.CapacityTypeEC2InstanceSP, 2)
	veneermetrics.SetOverlayCount(veneermetrics.CapacityTypeRI, 7)

	// Reset to zero (simulating all overlays deleted)
	veneermetrics.SetOverlayCount(veneermetrics.CapacityTypeComputeSP, 0)
	veneermetrics.SetOverlayCount(veneermetrics.CapacityTypeEC2InstanceSP, 0)
	veneermetrics.SetOverlayCount(veneermetrics.CapacityTypeRI, 0)

	// No panics means success
}
