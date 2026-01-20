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
	"strconv"
	"time"

	"github.com/nextdoor/veneer/pkg/overlay"
)

// Version is set at build time via ldflags.
var Version = "dev"

// Recorder provides a clean interface for recording Veneer metrics.
// It wraps the raw Prometheus metrics with helper methods that handle
// label mapping and type conversions.
type Recorder struct {
	// DisabledMode indicates whether the controller is running in disabled mode.
	// This is recorded as a label on the info metric.
	DisabledMode bool
}

// NewRecorder creates a new Recorder with the given configuration.
func NewRecorder(disabledMode bool) *Recorder {
	r := &Recorder{
		DisabledMode: disabledMode,
	}
	// Record the info metric once at startup
	r.recordInfo()
	return r
}

// recordInfo sets the info gauge with version and mode labels.
func (r *Recorder) recordInfo() {
	Info.WithLabelValues(Version, strconv.FormatBool(r.DisabledMode)).Set(1)
}

// ReconciliationTimer returns a function that should be called when
// reconciliation completes. It records the duration automatically.
//
// Usage:
//
//	done := recorder.ReconciliationTimer()
//	defer done()
func (r *Recorder) ReconciliationTimer() func() {
	start := time.Now()
	return func() {
		ReconciliationDuration.Observe(time.Since(start).Seconds())
	}
}

// RecordReconciliationSuccess increments the reconciliation counter for success.
func (r *Recorder) RecordReconciliationSuccess() {
	ReconciliationTotal.WithLabelValues(StatusSuccess).Inc()
}

// RecordReconciliationError increments the reconciliation counter for errors.
func (r *Recorder) RecordReconciliationError() {
	ReconciliationTotal.WithLabelValues(StatusError).Inc()
}

// RecordOverlayCreated records a successful overlay creation.
func (r *Recorder) RecordOverlayCreated(capacityType string) {
	OverlayOperationsTotal.WithLabelValues(OperationCreate, capacityType).Inc()
}

// RecordOverlayUpdated records a successful overlay update.
func (r *Recorder) RecordOverlayUpdated(capacityType string) {
	OverlayOperationsTotal.WithLabelValues(OperationUpdate, capacityType).Inc()
}

// RecordOverlayDeleted records a successful overlay deletion.
func (r *Recorder) RecordOverlayDeleted(capacityType string) {
	OverlayOperationsTotal.WithLabelValues(OperationDelete, capacityType).Inc()
}

// RecordOverlayError records an overlay operation error.
func (r *Recorder) RecordOverlayError(capacityType string) {
	OverlayErrorsTotal.WithLabelValues(capacityType).Inc()
}

// RecordActiveOverlays sets the gauge for active overlays by capacity type.
func (r *Recorder) RecordActiveOverlays(capacityType string, count int) {
	OverlaysActive.WithLabelValues(capacityType).Set(float64(count))
}

// RecordDataFreshness records the age of Lumina data in seconds.
func (r *Recorder) RecordDataFreshness(ageSeconds float64) {
	DataFreshnessSeconds.Set(ageSeconds)
}

// RecordDecision records a decision made by the decision engine.
func (r *Recorder) RecordDecision(decision overlay.Decision) {
	capacityType := capacityTypeToLabel(decision.CapacityType)
	shouldExist := strconv.FormatBool(decision.ShouldExist)
	DecisionsTotal.WithLabelValues(capacityType, shouldExist).Inc()
}

// RecordSavingsPlanMetrics records utilization and remaining capacity for a Savings Plan.
func (r *Recorder) RecordSavingsPlanMetrics(
	spType string,
	instanceFamily string,
	region string,
	utilizationPercent float64,
	remainingCapacity float64,
) {
	// Use empty string for global compute SPs
	if instanceFamily == "" {
		instanceFamily = "all"
	}
	if region == "" {
		region = "global"
	}

	SavingsPlanUtilizationPercent.WithLabelValues(spType, instanceFamily, region).Set(utilizationPercent)
	SavingsPlanRemainingCapacityDollars.WithLabelValues(spType, instanceFamily, region).Set(remainingCapacity)
}

// RecordReservedInstances records the count of Reserved Instances.
func (r *Recorder) RecordReservedInstances(instanceType, region string, count int) {
	ReservedInstancesTotal.WithLabelValues(instanceType, region).Set(float64(count))
}

// capacityTypeToLabel converts an overlay.CapacityType to a metric label string.
func capacityTypeToLabel(ct overlay.CapacityType) string {
	switch ct {
	case overlay.CapacityTypeComputeSavingsPlan:
		return CapacityTypeComputeSP
	case overlay.CapacityTypeEC2InstanceSavingsPlan:
		return CapacityTypeEC2InstanceSP
	case overlay.CapacityTypeReservedInstance:
		return CapacityTypeRI
	default:
		return string(ct)
	}
}

// CapacityTypeLabel returns the metric label for a given capacity type string.
// This is useful for callers that have the capacity type as a string.
func CapacityTypeLabel(capacityType string) string {
	switch capacityType {
	case string(overlay.CapacityTypeComputeSavingsPlan):
		return CapacityTypeComputeSP
	case string(overlay.CapacityTypeEC2InstanceSavingsPlan):
		return CapacityTypeEC2InstanceSP
	case string(overlay.CapacityTypeReservedInstance):
		return CapacityTypeRI
	default:
		return capacityType
	}
}
