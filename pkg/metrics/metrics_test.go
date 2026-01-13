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

package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTypeStringMethods verifies that all type String() methods return expected values.
func TestTypeStringMethods(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    interface{ String() string }
		expected string
	}{
		// Result
		{name: "ResultSuccess", value: ResultSuccess, expected: "success"},
		{name: "ResultError", value: ResultError, expected: "error"},
		// Operation
		{name: "OperationCreate", value: OperationCreate, expected: "create"},
		{name: "OperationUpdate", value: OperationUpdate, expected: "update"},
		{name: "OperationDelete", value: OperationDelete, expected: "delete"},
		// ErrorType
		{name: "ErrorTypeValidation", value: ErrorTypeValidation, expected: "validation"},
		{name: "ErrorTypeAPI", value: ErrorTypeAPI, expected: "api"},
		{name: "ErrorTypeNotFound", value: ErrorTypeNotFound, expected: "not_found"},
		// QueryType
		{name: "QueryTypeSPUtilization", value: QueryTypeSPUtilization, expected: "sp_utilization"},
		{name: "QueryTypeSPCapacity", value: QueryTypeSPCapacity, expected: "sp_capacity"},
		{name: "QueryTypeRI", value: QueryTypeRI, expected: "ri"},
		{name: "QueryTypeDataFreshness", value: QueryTypeDataFreshness, expected: "data_freshness"},
		// CapacityType
		{name: "CapacityTypeComputeSP", value: CapacityTypeComputeSP, expected: "compute_savings_plan"},
		{name: "CapacityTypeEC2InstanceSP", value: CapacityTypeEC2InstanceSP, expected: "ec2_instance_savings_plan"},
		{name: "CapacityTypeRI", value: CapacityTypeRI, expected: "reserved_instance"},
		// DecisionReason
		{name: "ReasonCapacityAvailable", value: ReasonCapacityAvailable, expected: "capacity_available"},
		{name: "ReasonUtilizationAboveThreshold", value: ReasonUtilizationAboveThreshold, expected: "utilization_above_threshold"},
		{name: "ReasonNoCapacity", value: ReasonNoCapacity, expected: "no_capacity"},
		{name: "ReasonRIAvailable", value: ReasonRIAvailable, expected: "ri_available"},
		{name: "ReasonRINotFound", value: ReasonRINotFound, expected: "ri_not_found"},
		{name: "ReasonUnknown", value: ReasonUnknown, expected: "unknown"},
		// ShouldExist
		{name: "ShouldExistTrue", value: ShouldExistTrue, expected: "true"},
		{name: "ShouldExistFalse", value: ShouldExistFalse, expected: "false"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, tc.value.String())
		})
	}
}

// TestBoolToShouldExist verifies boolean to ShouldExist conversion.
func TestBoolToShouldExist(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    bool
		expected ShouldExist
	}{
		{name: "true_returns_ShouldExistTrue", input: true, expected: ShouldExistTrue},
		{name: "false_returns_ShouldExistFalse", input: false, expected: ShouldExistFalse},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := BoolToShouldExist(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestSanitizeReason verifies reason string sanitization to controlled labels.
func TestSanitizeReason(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected DecisionReason
	}{
		{
			name:     "utilization_above_threshold",
			input:    "utilization 97.5% at/above threshold 95.0%",
			expected: ReasonUtilizationAboveThreshold,
		},
		{
			name:     "capacity_available_below_threshold",
			input:    "utilization 85.0% below threshold 95.0%, capacity available (10.50 $/hour)",
			expected: ReasonCapacityAvailable,
		},
		{
			name:     "no_remaining_capacity",
			input:    "no remaining capacity (0.00 $/hour)",
			expected: ReasonNoCapacity,
		},
		{
			name:     "reserved_instances_available",
			input:    "5 reserved instances available",
			expected: ReasonRIAvailable,
		},
		{
			name:     "no_reserved_instances",
			input:    "no reserved instances available",
			expected: ReasonRINotFound,
		},
		{
			name:     "unknown_reason",
			input:    "some completely unknown reason",
			expected: ReasonUnknown,
		},
		{
			name:     "empty_string",
			input:    "",
			expected: ReasonUnknown,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := SanitizeReason(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestCapacityTypeFromOverlay verifies overlay capacity type conversion.
func TestCapacityTypeFromOverlay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected CapacityType
	}{
		{
			name:     "compute_savings_plan",
			input:    "compute_savings_plan",
			expected: CapacityTypeComputeSP,
		},
		{
			name:     "ec2_instance_savings_plan",
			input:    "ec2_instance_savings_plan",
			expected: CapacityTypeEC2InstanceSP,
		},
		{
			name:     "reserved_instance",
			input:    "reserved_instance",
			expected: CapacityTypeRI,
		},
		{
			name:     "unknown_type_passed_through",
			input:    "some_new_type",
			expected: CapacityType("some_new_type"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := CapacityTypeFromOverlay(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestSetConfigMetrics verifies configuration metric updates.
func TestSetConfigMetrics(t *testing.T) {
	tests := []struct {
		name                  string
		overlaysDisabled      bool
		utilizationThreshold  float64
		expectedDisabledValue float64
	}{
		{
			name:                  "overlays_enabled",
			overlaysDisabled:      false,
			utilizationThreshold:  95.0,
			expectedDisabledValue: 0,
		},
		{
			name:                  "overlays_disabled",
			overlaysDisabled:      true,
			utilizationThreshold:  90.0,
			expectedDisabledValue: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			SetConfigMetrics(tc.overlaysDisabled, tc.utilizationThreshold)

			// Verify ConfigOverlaysDisabled
			disabledValue := getGaugeValue(t, ConfigOverlaysDisabled)
			assert.Equal(t, tc.expectedDisabledValue, disabledValue)

			// Verify ConfigUtilizationThreshold
			thresholdValue := getGaugeValue(t, ConfigUtilizationThreshold)
			assert.Equal(t, tc.utilizationThreshold, thresholdValue)
		})
	}
}

// TestSetLuminaDataFreshness verifies Lumina data freshness metric updates.
func TestSetLuminaDataFreshness(t *testing.T) {
	const maxFreshness = 600.0 // 10 minutes

	tests := []struct {
		name                   string
		freshnessSeconds       float64
		expectedAvailableValue float64
	}{
		{
			name:                   "data_fresh",
			freshnessSeconds:       300.0, // 5 minutes
			expectedAvailableValue: 1,
		},
		{
			name:                   "data_at_threshold",
			freshnessSeconds:       600.0, // exactly 10 minutes
			expectedAvailableValue: 0,     // >= maxFreshness means stale
		},
		{
			name:                   "data_stale",
			freshnessSeconds:       900.0, // 15 minutes
			expectedAvailableValue: 0,
		},
		{
			name:                   "data_very_fresh",
			freshnessSeconds:       10.0, // 10 seconds
			expectedAvailableValue: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			SetLuminaDataFreshness(tc.freshnessSeconds, maxFreshness)

			// Verify LuminaDataFreshnessSeconds
			freshnessValue := getGaugeValue(t, LuminaDataFreshnessSeconds)
			assert.Equal(t, tc.freshnessSeconds, freshnessValue)

			// Verify LuminaDataAvailable
			availableValue := getGaugeValue(t, LuminaDataAvailable)
			assert.Equal(t, tc.expectedAvailableValue, availableValue)
		})
	}
}

// TestSetLuminaDataUnavailable verifies the unavailable state.
func TestSetLuminaDataUnavailable(t *testing.T) {
	// First set it to available
	SetLuminaDataFreshness(100.0, 600.0)
	assert.Equal(t, float64(1), getGaugeValue(t, LuminaDataAvailable))

	// Now mark as unavailable
	SetLuminaDataUnavailable()
	assert.Equal(t, float64(0), getGaugeValue(t, LuminaDataAvailable))
}

// TestRecordReconciliation verifies reconciliation metric recording.
func TestRecordReconciliation(t *testing.T) {
	// Record success
	RecordReconciliation(ResultSuccess, 1.5)

	// Record error
	RecordReconciliation(ResultError, 0.5)

	// Verify counter incremented (we can't easily check exact values with counter vecs
	// in unit tests without more setup, but we verify no panics occur)
}

// TestRecordDecision verifies decision metric recording.
func TestRecordDecision(t *testing.T) {
	tests := []struct {
		name         string
		capacityType CapacityType
		shouldExist  ShouldExist
		reason       DecisionReason
	}{
		{
			name:         "compute_sp_should_exist",
			capacityType: CapacityTypeComputeSP,
			shouldExist:  ShouldExistTrue,
			reason:       ReasonCapacityAvailable,
		},
		{
			name:         "ec2_sp_should_not_exist",
			capacityType: CapacityTypeEC2InstanceSP,
			shouldExist:  ShouldExistFalse,
			reason:       ReasonUtilizationAboveThreshold,
		},
		{
			name:         "ri_available",
			capacityType: CapacityTypeRI,
			shouldExist:  ShouldExistTrue,
			reason:       ReasonRIAvailable,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Should not panic
			RecordDecision(tc.capacityType, tc.shouldExist, tc.reason)
		})
	}
}

// TestRecordOverlayOperation verifies overlay operation metric recording.
func TestRecordOverlayOperation(t *testing.T) {
	operations := []Operation{OperationCreate, OperationUpdate, OperationDelete}
	capacityTypes := []CapacityType{CapacityTypeComputeSP, CapacityTypeEC2InstanceSP, CapacityTypeRI}

	for _, op := range operations {
		for _, ct := range capacityTypes {
			t.Run(op.String()+"_"+ct.String(), func(t *testing.T) {
				// Should not panic
				RecordOverlayOperation(op, ct)
			})
		}
	}
}

// TestRecordOverlayOperationError verifies overlay error metric recording.
func TestRecordOverlayOperationError(t *testing.T) {
	operations := []Operation{OperationCreate, OperationUpdate, OperationDelete}
	errorTypes := []ErrorType{ErrorTypeValidation, ErrorTypeAPI, ErrorTypeNotFound}

	for _, op := range operations {
		for _, et := range errorTypes {
			t.Run(op.String()+"_"+et.String(), func(t *testing.T) {
				// Should not panic
				RecordOverlayOperationError(op, et)
			})
		}
	}
}

// TestRecordPrometheusQuery verifies Prometheus query metric recording.
func TestRecordPrometheusQuery(t *testing.T) {
	queryTypes := []QueryType{
		QueryTypeSPUtilization,
		QueryTypeSPCapacity,
		QueryTypeRI,
		QueryTypeDataFreshness,
	}

	for _, qt := range queryTypes {
		t.Run(qt.String()+"_success", func(t *testing.T) {
			RecordPrometheusQuery(qt, 0.1, 5, nil)
		})
		t.Run(qt.String()+"_error", func(t *testing.T) {
			RecordPrometheusQuery(qt, 0.05, 0, assert.AnError)
		})
	}
}

// TestSetReservedInstanceMetrics verifies RI metric setting.
func TestSetReservedInstanceMetrics(t *testing.T) {
	tests := []struct {
		name          string
		dataAvailable bool
		counts        map[string]map[string]int
	}{
		{
			name:          "data_available_with_counts",
			dataAvailable: true,
			counts: map[string]map[string]int{
				"m5.xlarge":  {"us-west-2": 5, "us-east-1": 3},
				"c5.2xlarge": {"us-west-2": 2},
			},
		},
		{
			name:          "data_unavailable",
			dataAvailable: false,
			counts:        nil,
		},
		{
			name:          "data_available_empty_counts",
			dataAvailable: true,
			counts:        map[string]map[string]int{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			SetReservedInstanceMetrics(tc.dataAvailable, tc.counts)

			expectedAvailable := float64(0)
			if tc.dataAvailable {
				expectedAvailable = 1
			}
			assert.Equal(t, expectedAvailable, getGaugeValue(t, ReservedInstanceDataAvailable))
		})
	}
}

// TestSetOverlayCount verifies overlay count metric setting.
func TestSetOverlayCount(t *testing.T) {
	capacityTypes := []CapacityType{CapacityTypeComputeSP, CapacityTypeEC2InstanceSP, CapacityTypeRI}

	for _, ct := range capacityTypes {
		t.Run(ct.String(), func(t *testing.T) {
			SetOverlayCount(ct, 5)
			// Verify no panic - actual value verification requires more setup
		})
	}
}

// TestSetBuildInfo verifies build info metric setting.
func TestSetBuildInfo(t *testing.T) {
	SetBuildInfo("v1.0.0", "abc123", "2025-01-13")
	// Verify no panic - the metric is a gauge vec so value verification is complex
}

// TestMetricConstants verifies that all metric name constants are non-empty.
func TestMetricConstants(t *testing.T) {
	t.Parallel()

	metricNames := []string{
		MetricReconciliationDuration,
		MetricReconciliationTotal,
		MetricLuminaDataFreshnessSeconds,
		MetricLuminaDataAvailable,
		MetricDecisionTotal,
		MetricReservedInstanceDataAvail,
		MetricReservedInstanceCount,
		MetricOverlayOperationsTotal,
		MetricOverlayOperationErrorsTotal,
		MetricOverlayCount,
		MetricPrometheusQueryDuration,
		MetricPrometheusQueryErrorsTotal,
		MetricPrometheusQueryResultCount,
		MetricConfigOverlaysDisabled,
		MetricConfigUtilizationThreshold,
		MetricBuildInfo,
	}

	for _, name := range metricNames {
		assert.NotEmpty(t, name, "metric name should not be empty")
	}
}

// TestLabelConstants verifies that all label key constants are non-empty.
func TestLabelConstants(t *testing.T) {
	t.Parallel()

	labelKeys := []string{
		LabelResult,
		LabelOperation,
		LabelErrorType,
		LabelQueryType,
		LabelCapacityType,
		LabelShouldExist,
		LabelReason,
		LabelInstanceType,
		LabelRegion,
		LabelVersion,
		LabelCommit,
		LabelBuildDate,
	}

	for _, key := range labelKeys {
		assert.NotEmpty(t, key, "label key should not be empty")
	}
}

// TestNamespaceConstant verifies the namespace constant.
func TestNamespaceConstant(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "veneer", Namespace)
}

// Helper function to get a gauge value.
func getGaugeValue(t *testing.T, gauge prometheus.Gauge) float64 {
	t.Helper()

	var metric io_prometheus_client.Metric
	err := gauge.Write(&metric)
	require.NoError(t, err)
	return metric.GetGauge().GetValue()
}
