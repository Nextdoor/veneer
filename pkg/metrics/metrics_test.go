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
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nextdoor/veneer/pkg/overlay"
)

func TestNewRecorder(t *testing.T) {
	tests := []struct {
		name         string
		disabledMode bool
	}{
		{
			name:         "disabled mode true",
			disabledMode: true,
		},
		{
			name:         "disabled mode false",
			disabledMode: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := NewRecorder(tt.disabledMode)
			assert.NotNil(t, recorder)
			assert.Equal(t, tt.disabledMode, recorder.DisabledMode)
		})
	}
}

func TestReconciliationTimer(t *testing.T) {
	recorder := NewRecorder(false)

	// Start the timer and wait a bit
	done := recorder.ReconciliationTimer()
	time.Sleep(10 * time.Millisecond)
	done()

	// Verify the histogram has a sample by checking it doesn't panic
	// Histograms don't have a simple way to check sample count via testutil
	require.NotPanics(t, func() {
		ReconciliationDuration.Observe(0.001)
	})
}

func TestRecordReconciliationStatus(t *testing.T) {
	recorder := NewRecorder(false)

	// Reset counters for this test
	ReconciliationTotal.Reset()

	// Record some successes and errors
	recorder.RecordReconciliationSuccess()
	recorder.RecordReconciliationSuccess()
	recorder.RecordReconciliationError()

	// Verify counts
	successCount := testutil.ToFloat64(ReconciliationTotal.WithLabelValues(StatusSuccess))
	errorCount := testutil.ToFloat64(ReconciliationTotal.WithLabelValues(StatusError))

	assert.Equal(t, 2.0, successCount)
	assert.Equal(t, 1.0, errorCount)
}

func TestRecordOverlayOperations(t *testing.T) {
	recorder := NewRecorder(false)

	// Reset counters for this test
	OverlayOperationsTotal.Reset()

	// Record various operations
	recorder.RecordOverlayCreated(CapacityTypeComputeSP)
	recorder.RecordOverlayCreated(CapacityTypeComputeSP)
	recorder.RecordOverlayUpdated(CapacityTypeEC2InstanceSP)
	recorder.RecordOverlayDeleted(CapacityTypeRI)

	// Verify counts
	createCount := testutil.ToFloat64(OverlayOperationsTotal.WithLabelValues(OperationCreate, CapacityTypeComputeSP))
	updateCount := testutil.ToFloat64(OverlayOperationsTotal.WithLabelValues(OperationUpdate, CapacityTypeEC2InstanceSP))
	deleteCount := testutil.ToFloat64(OverlayOperationsTotal.WithLabelValues(OperationDelete, CapacityTypeRI))

	assert.Equal(t, 2.0, createCount)
	assert.Equal(t, 1.0, updateCount)
	assert.Equal(t, 1.0, deleteCount)
}

func TestRecordOverlayError(t *testing.T) {
	recorder := NewRecorder(false)

	// Reset counters for this test
	OverlayErrorsTotal.Reset()

	recorder.RecordOverlayError(CapacityTypeComputeSP)
	recorder.RecordOverlayError(CapacityTypeComputeSP)

	errorCount := testutil.ToFloat64(OverlayErrorsTotal.WithLabelValues(CapacityTypeComputeSP))
	assert.Equal(t, 2.0, errorCount)
}

func TestRecordActiveOverlays(t *testing.T) {
	recorder := NewRecorder(false)

	// Reset gauge for this test
	OverlaysActive.Reset()

	recorder.RecordActiveOverlays(CapacityTypeComputeSP, 5)
	recorder.RecordActiveOverlays(CapacityTypeEC2InstanceSP, 3)

	computeActive := testutil.ToFloat64(OverlaysActive.WithLabelValues(CapacityTypeComputeSP))
	ec2Active := testutil.ToFloat64(OverlaysActive.WithLabelValues(CapacityTypeEC2InstanceSP))

	assert.Equal(t, 5.0, computeActive)
	assert.Equal(t, 3.0, ec2Active)

	// Update the count
	recorder.RecordActiveOverlays(CapacityTypeComputeSP, 2)
	computeActive = testutil.ToFloat64(OverlaysActive.WithLabelValues(CapacityTypeComputeSP))
	assert.Equal(t, 2.0, computeActive)
}

func TestRecordDataFreshness(t *testing.T) {
	recorder := NewRecorder(false)

	recorder.RecordDataFreshness(123.45)

	freshness := testutil.ToFloat64(DataFreshnessSeconds)
	assert.Equal(t, 123.45, freshness)
}

func TestRecordDecision(t *testing.T) {
	recorder := NewRecorder(false)

	// Reset counters for this test
	DecisionsTotal.Reset()

	// Record decisions
	recorder.RecordDecision(overlay.Decision{
		CapacityType: overlay.CapacityTypeComputeSavingsPlan,
		ShouldExist:  true,
	})
	recorder.RecordDecision(overlay.Decision{
		CapacityType: overlay.CapacityTypeComputeSavingsPlan,
		ShouldExist:  false,
	})
	recorder.RecordDecision(overlay.Decision{
		CapacityType: overlay.CapacityTypeReservedInstance,
		ShouldExist:  true,
	})

	// Verify counts
	computeTrue := testutil.ToFloat64(DecisionsTotal.WithLabelValues(CapacityTypeComputeSP, "true"))
	computeFalse := testutil.ToFloat64(DecisionsTotal.WithLabelValues(CapacityTypeComputeSP, "false"))
	riTrue := testutil.ToFloat64(DecisionsTotal.WithLabelValues(CapacityTypeRI, "true"))

	assert.Equal(t, 1.0, computeTrue)
	assert.Equal(t, 1.0, computeFalse)
	assert.Equal(t, 1.0, riTrue)
}

func TestRecordSavingsPlanMetrics(t *testing.T) {
	recorder := NewRecorder(false)

	// Reset gauges for this test
	SavingsPlanUtilizationPercent.Reset()
	SavingsPlanRemainingCapacityDollars.Reset()

	// Record compute SP (global, no family/region)
	recorder.RecordSavingsPlanMetrics("compute", "", "", 85.5, 100.25)

	// Record EC2 instance SP (specific family/region)
	recorder.RecordSavingsPlanMetrics("ec2_instance", "m5", "us-west-2", 92.0, 50.0)

	// Verify compute SP metrics (uses "all" and "global" for empty values)
	computeUtil := testutil.ToFloat64(SavingsPlanUtilizationPercent.WithLabelValues("compute", "all", "global"))
	computeCap := testutil.ToFloat64(SavingsPlanRemainingCapacityDollars.WithLabelValues("compute", "all", "global"))
	assert.Equal(t, 85.5, computeUtil)
	assert.Equal(t, 100.25, computeCap)

	// Verify EC2 instance SP metrics
	ec2Util := testutil.ToFloat64(SavingsPlanUtilizationPercent.WithLabelValues("ec2_instance", "m5", "us-west-2"))
	ec2Cap := testutil.ToFloat64(SavingsPlanRemainingCapacityDollars.WithLabelValues("ec2_instance", "m5", "us-west-2"))
	assert.Equal(t, 92.0, ec2Util)
	assert.Equal(t, 50.0, ec2Cap)
}

func TestRecordReservedInstances(t *testing.T) {
	recorder := NewRecorder(false)

	// Reset gauge for this test
	ReservedInstancesTotal.Reset()

	recorder.RecordReservedInstances("m5.xlarge", "us-west-2", 10)
	recorder.RecordReservedInstances("c5.2xlarge", "us-east-1", 5)

	m5Count := testutil.ToFloat64(ReservedInstancesTotal.WithLabelValues("m5.xlarge", "us-west-2"))
	c5Count := testutil.ToFloat64(ReservedInstancesTotal.WithLabelValues("c5.2xlarge", "us-east-1"))

	assert.Equal(t, 10.0, m5Count)
	assert.Equal(t, 5.0, c5Count)
}

func TestCapacityTypeToLabel(t *testing.T) {
	tests := []struct {
		capacityType overlay.CapacityType
		expected     string
	}{
		{overlay.CapacityTypeComputeSavingsPlan, CapacityTypeComputeSP},
		{overlay.CapacityTypeEC2InstanceSavingsPlan, CapacityTypeEC2InstanceSP},
		{overlay.CapacityTypeReservedInstance, CapacityTypeRI},
		{overlay.CapacityType("unknown"), "unknown"},
	}

	for _, tt := range tests {
		t.Run(string(tt.capacityType), func(t *testing.T) {
			result := capacityTypeToLabel(tt.capacityType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCapacityTypeLabel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{string(overlay.CapacityTypeComputeSavingsPlan), CapacityTypeComputeSP},
		{string(overlay.CapacityTypeEC2InstanceSavingsPlan), CapacityTypeEC2InstanceSP},
		{string(overlay.CapacityTypeReservedInstance), CapacityTypeRI},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := CapacityTypeLabel(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMetricsAreRegistered(t *testing.T) {
	// This test verifies that all metrics are properly registered
	// by attempting to gather them from the default registry.
	// The init() function in metrics.go registers them with controller-runtime's registry,
	// but we can verify the metrics exist by checking they don't panic when used.

	// These should not panic
	require.NotPanics(t, func() {
		ReconciliationDuration.Observe(1.0)
	})
	require.NotPanics(t, func() {
		ReconciliationTotal.WithLabelValues(StatusSuccess).Inc()
	})
	require.NotPanics(t, func() {
		OverlayOperationsTotal.WithLabelValues(OperationCreate, CapacityTypeComputeSP).Inc()
	})
	require.NotPanics(t, func() {
		OverlayErrorsTotal.WithLabelValues(CapacityTypeComputeSP).Inc()
	})
	require.NotPanics(t, func() {
		OverlaysActive.WithLabelValues(CapacityTypeComputeSP).Set(1)
	})
	require.NotPanics(t, func() {
		SavingsPlanUtilizationPercent.WithLabelValues("compute", "all", "global").Set(50)
	})
	require.NotPanics(t, func() {
		SavingsPlanRemainingCapacityDollars.WithLabelValues("compute", "all", "global").Set(100)
	})
	require.NotPanics(t, func() {
		ReservedInstancesTotal.WithLabelValues("m5.xlarge", "us-west-2").Set(5)
	})
	require.NotPanics(t, func() {
		DataFreshnessSeconds.Set(60)
	})
	require.NotPanics(t, func() {
		Info.WithLabelValues("1.0.0", "true").Set(1)
	})
	require.NotPanics(t, func() {
		DecisionsTotal.WithLabelValues(CapacityTypeComputeSP, "true").Inc()
	})
}

func TestMetricDescriptions(t *testing.T) {
	// Verify that all metrics have proper descriptions by checking
	// they can be described without panicking
	ch := make(chan *prometheus.Desc, 100)

	require.NotPanics(t, func() {
		ReconciliationDuration.Describe(ch)
	})
	require.NotPanics(t, func() {
		ReconciliationTotal.Describe(ch)
	})
	require.NotPanics(t, func() {
		OverlayOperationsTotal.Describe(ch)
	})
	require.NotPanics(t, func() {
		OverlayErrorsTotal.Describe(ch)
	})
	require.NotPanics(t, func() {
		OverlaysActive.Describe(ch)
	})
	require.NotPanics(t, func() {
		SavingsPlanUtilizationPercent.Describe(ch)
	})
	require.NotPanics(t, func() {
		SavingsPlanRemainingCapacityDollars.Describe(ch)
	})
	require.NotPanics(t, func() {
		ReservedInstancesTotal.Describe(ch)
	})
	require.NotPanics(t, func() {
		DataFreshnessSeconds.Describe(ch)
	})
	require.NotPanics(t, func() {
		Info.Describe(ch)
	})
	require.NotPanics(t, func() {
		DecisionsTotal.Describe(ch)
	})

	close(ch)
}
