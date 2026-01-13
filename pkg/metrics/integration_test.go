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
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	veneermetrics "github.com/nextdoor/veneer/pkg/metrics"
)

// newTestMetrics creates a Metrics instance with a new registry for testing.
// Each test gets its own registry to avoid metric registration conflicts.
func newTestMetrics(t *testing.T) *veneermetrics.Metrics {
	t.Helper()
	reg := prometheus.NewRegistry()
	return veneermetrics.NewMetrics(reg)
}

// TestMetricsIntegration_ReconciliationWorkflow tests the full reconciliation metrics workflow.
// This simulates what happens during a real reconciliation cycle:
// 1. Record reconciliation start
// 2. Record Prometheus queries
// 3. Record decisions made
// 4. Record overlay operations
// 5. Record reconciliation completion
func TestMetricsIntegration_ReconciliationWorkflow(t *testing.T) {
	m := newTestMetrics(t)

	// Step 1: Set configuration metrics (done at startup)
	m.SetConfigMetrics(false, 95.0)

	// Step 2: Query data freshness
	m.RecordPrometheusQuery(veneermetrics.QueryTypeDataFreshness, 0.05, 1, nil)
	m.SetLuminaDataFreshness(30.0, 600.0) // 30 seconds old, max 600 seconds

	// Step 3: Query Savings Plan utilization
	m.RecordPrometheusQuery(veneermetrics.QueryTypeSPUtilization, 0.1, 3, nil)

	// Step 4: Query Savings Plan capacity
	m.RecordPrometheusQuery(veneermetrics.QueryTypeSPCapacity, 0.08, 2, nil)

	// Step 5: Query Reserved Instances
	m.RecordPrometheusQuery(veneermetrics.QueryTypeRI, 0.06, 5, nil)

	// Step 6: Record decisions
	m.RecordDecision(
		veneermetrics.CapacityTypeComputeSP,
		veneermetrics.ShouldExistTrue,
		veneermetrics.ReasonCapacityAvailable,
	)
	m.RecordDecision(
		veneermetrics.CapacityTypeEC2InstanceSP,
		veneermetrics.ShouldExistFalse,
		veneermetrics.ReasonUtilizationAboveThreshold,
	)
	m.RecordDecision(
		veneermetrics.CapacityTypeRI,
		veneermetrics.ShouldExistTrue,
		veneermetrics.ReasonRIAvailable,
	)

	// Step 7: Record overlay operations
	m.RecordOverlayOperation(veneermetrics.OperationCreate, veneermetrics.CapacityTypeComputeSP)
	m.RecordOverlayOperation(veneermetrics.OperationDelete, veneermetrics.CapacityTypeEC2InstanceSP)
	m.RecordOverlayOperation(veneermetrics.OperationUpdate, veneermetrics.CapacityTypeRI)

	// Step 8: Set overlay counts
	m.SetOverlayCount(veneermetrics.CapacityTypeComputeSP, 1)
	m.SetOverlayCount(veneermetrics.CapacityTypeEC2InstanceSP, 0)
	m.SetOverlayCount(veneermetrics.CapacityTypeRI, 2)

	// Step 9: Record successful reconciliation
	m.RecordReconciliation(veneermetrics.ResultSuccess, 0.5)

	// Verify key metrics are recorded correctly
	t.Run("lumina_data_available_is_set", func(t *testing.T) {
		value := testutil.ToFloat64(m.LuminaDataAvailable)
		assert.Equal(t, float64(1), value, "Lumina data should be available")
	})

	t.Run("lumina_data_freshness_is_set", func(t *testing.T) {
		value := testutil.ToFloat64(m.LuminaDataFreshnessSeconds)
		assert.Equal(t, 30.0, value, "Lumina data freshness should be 30 seconds")
	})

	t.Run("config_overlays_disabled_is_set", func(t *testing.T) {
		value := testutil.ToFloat64(m.ConfigOverlaysDisabled)
		assert.Equal(t, float64(0), value, "Overlays should not be disabled")
	})

	t.Run("config_utilization_threshold_is_set", func(t *testing.T) {
		value := testutil.ToFloat64(m.ConfigUtilizationThreshold)
		assert.Equal(t, 95.0, value, "Utilization threshold should be 95%")
	})
}

// TestMetricsIntegration_ErrorWorkflow tests metrics recording during error scenarios.
func TestMetricsIntegration_ErrorWorkflow(t *testing.T) {
	m := newTestMetrics(t)

	// Query fails
	m.RecordPrometheusQuery(veneermetrics.QueryTypeDataFreshness, 0.5, 0, assert.AnError)
	m.SetLuminaDataUnavailable()

	// Overlay operation fails
	m.RecordOverlayOperationError(veneermetrics.OperationCreate, veneermetrics.ErrorTypeAPI)
	m.RecordOverlayOperationError(veneermetrics.OperationUpdate, veneermetrics.ErrorTypeValidation)
	m.RecordOverlayOperationError(veneermetrics.OperationDelete, veneermetrics.ErrorTypeNotFound)

	// Record failed reconciliation
	m.RecordReconciliation(veneermetrics.ResultError, 1.0)

	// Verify Lumina data is marked unavailable
	t.Run("lumina_data_unavailable", func(t *testing.T) {
		value := testutil.ToFloat64(m.LuminaDataAvailable)
		assert.Equal(t, float64(0), value, "Lumina data should be unavailable")
	})
}

// TestMetricsIntegration_ReservedInstanceMetrics tests RI-specific metrics.
func TestMetricsIntegration_ReservedInstanceMetrics(t *testing.T) {
	m := newTestMetrics(t)

	// Simulate RI data being available with counts
	riCounts := map[string]map[string]int{
		"m5.xlarge":  {"us-west-2": 5, "us-east-1": 3},
		"c5.2xlarge": {"us-west-2": 2},
	}

	m.SetReservedInstanceMetrics(true, riCounts)

	t.Run("ri_data_available", func(t *testing.T) {
		value := testutil.ToFloat64(m.ReservedInstanceDataAvailable)
		assert.Equal(t, float64(1), value, "RI data should be available")
	})

	// Test with no RI data
	m.SetReservedInstanceMetrics(false, nil)

	t.Run("ri_data_unavailable", func(t *testing.T) {
		value := testutil.ToFloat64(m.ReservedInstanceDataAvailable)
		assert.Equal(t, float64(0), value, "RI data should be unavailable")
	})
}

// TestMetricsIntegration_DisabledMode tests metrics when overlays are disabled.
func TestMetricsIntegration_DisabledMode(t *testing.T) {
	m := newTestMetrics(t)

	// Set disabled mode
	m.SetConfigMetrics(true, 90.0)

	t.Run("config_overlays_disabled_enabled", func(t *testing.T) {
		value := testutil.ToFloat64(m.ConfigOverlaysDisabled)
		assert.Equal(t, float64(1), value, "Overlays should be disabled")
	})

	t.Run("config_utilization_threshold_custom", func(t *testing.T) {
		value := testutil.ToFloat64(m.ConfigUtilizationThreshold)
		assert.Equal(t, 90.0, value, "Utilization threshold should be 90%")
	})

	// Reset to enabled
	m.SetConfigMetrics(false, 95.0)

	t.Run("config_overlays_re-enabled", func(t *testing.T) {
		value := testutil.ToFloat64(m.ConfigOverlaysDisabled)
		assert.Equal(t, float64(0), value, "Overlays should be enabled")
	})
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
			m := newTestMetrics(t)
			m.SetLuminaDataFreshness(tc.freshnessSeconds, maxFreshness)

			freshnessValue := testutil.ToFloat64(m.LuminaDataFreshnessSeconds)
			assert.Equal(t, tc.freshnessSeconds, freshnessValue)

			availableValue := testutil.ToFloat64(m.LuminaDataAvailable)
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
					m := newTestMetrics(t)
					// Should not panic
					m.RecordDecision(ct, se, r)
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
				m := newTestMetrics(t)
				m.RecordOverlayOperation(op, ct)
			})
		}
	}

	// Test error operations
	for _, op := range operations {
		for _, et := range errorTypes {
			t.Run("error_"+op.String()+"_"+et.String(), func(t *testing.T) {
				m := newTestMetrics(t)
				m.RecordOverlayOperationError(op, et)
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
			m := newTestMetrics(t)
			m.RecordPrometheusQuery(qt, 0.1, 5, nil)
		})
		t.Run("query_"+qt.String()+"_error", func(t *testing.T) {
			m := newTestMetrics(t)
			m.RecordPrometheusQuery(qt, 0.5, 0, assert.AnError)
		})
	}
}

// TestMetricsIntegration_MetricRegistration verifies all metrics are properly registered.
func TestMetricsIntegration_MetricRegistration(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := veneermetrics.NewMetrics(reg)

	// Gather all metrics to verify registration
	families, err := reg.Gather()
	require.NoError(t, err)

	// Build a map of metric names for easy lookup
	metricNames := make(map[string]bool)
	for _, mf := range families {
		metricNames[*mf.Name] = true
	}

	// Set some values so metrics appear in gather
	m.SetConfigMetrics(false, 95.0)
	m.SetLuminaDataFreshness(30.0, 600.0)
	m.SetReservedInstanceMetrics(true, nil)
	m.RecordReconciliation(veneermetrics.ResultSuccess, 0.1)

	// Re-gather after setting values
	families, err = reg.Gather()
	require.NoError(t, err)

	metricNames = make(map[string]bool)
	for _, mf := range families {
		metricNames[*mf.Name] = true
	}

	// Verify expected Veneer metrics are registered
	expectedMetrics := []string{
		"veneer_reconciliation_duration_seconds",
		"veneer_reconciliation_total",
		"veneer_lumina_data_freshness_seconds",
		"veneer_lumina_data_available",
		"veneer_reserved_instance_data_available",
		"veneer_config_overlays_disabled",
		"veneer_config_utilization_threshold_percent",
	}

	for _, name := range expectedMetrics {
		t.Run("registered_"+name, func(t *testing.T) {
			assert.True(t, metricNames[name], "metric %s should be registered", name)
		})
	}
}

// TestMetricsIntegration_OverlayCountByCapacityType tests overlay count tracking.
func TestMetricsIntegration_OverlayCountByCapacityType(t *testing.T) {
	m := newTestMetrics(t)

	// Set counts for each capacity type
	m.SetOverlayCount(veneermetrics.CapacityTypeComputeSP, 1)
	m.SetOverlayCount(veneermetrics.CapacityTypeEC2InstanceSP, 3)
	m.SetOverlayCount(veneermetrics.CapacityTypeRI, 5)

	// Update counts (simulating next reconciliation)
	m.SetOverlayCount(veneermetrics.CapacityTypeComputeSP, 1)
	m.SetOverlayCount(veneermetrics.CapacityTypeEC2InstanceSP, 2)
	m.SetOverlayCount(veneermetrics.CapacityTypeRI, 7)

	// Reset to zero (simulating all overlays deleted)
	m.SetOverlayCount(veneermetrics.CapacityTypeComputeSP, 0)
	m.SetOverlayCount(veneermetrics.CapacityTypeEC2InstanceSP, 0)
	m.SetOverlayCount(veneermetrics.CapacityTypeRI, 0)

	// No panics means success
}

// TestMetricsIntegration_NewMetricsCreatesAllFields verifies NewMetrics initializes all fields.
func TestMetricsIntegration_NewMetricsCreatesAllFields(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := veneermetrics.NewMetrics(reg)

	// Verify all fields are non-nil
	assert.NotNil(t, m.ReconciliationDuration, "ReconciliationDuration should not be nil")
	assert.NotNil(t, m.ReconciliationTotal, "ReconciliationTotal should not be nil")
	assert.NotNil(t, m.LuminaDataFreshnessSeconds, "LuminaDataFreshnessSeconds should not be nil")
	assert.NotNil(t, m.LuminaDataAvailable, "LuminaDataAvailable should not be nil")
	assert.NotNil(t, m.DecisionTotal, "DecisionTotal should not be nil")
	assert.NotNil(t, m.ReservedInstanceDataAvailable, "ReservedInstanceDataAvailable should not be nil")
	assert.NotNil(t, m.ReservedInstanceCount, "ReservedInstanceCount should not be nil")
	assert.NotNil(t, m.OverlayOperationsTotal, "OverlayOperationsTotal should not be nil")
	assert.NotNil(t, m.OverlayOperationErrorsTotal, "OverlayOperationErrorsTotal should not be nil")
	assert.NotNil(t, m.OverlayCount, "OverlayCount should not be nil")
	assert.NotNil(t, m.PrometheusQueryDuration, "PrometheusQueryDuration should not be nil")
	assert.NotNil(t, m.PrometheusQueryErrorsTotal, "PrometheusQueryErrorsTotal should not be nil")
	assert.NotNil(t, m.PrometheusQueryResultCount, "PrometheusQueryResultCount should not be nil")
	assert.NotNil(t, m.ConfigOverlaysDisabled, "ConfigOverlaysDisabled should not be nil")
	assert.NotNil(t, m.ConfigUtilizationThreshold, "ConfigUtilizationThreshold should not be nil")
}
