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

// Package metrics provides Prometheus metrics instrumentation for the Veneer controller.
//
// This package exposes metrics about Veneer's decision-making behavior, NodeOverlay lifecycle,
// and data source health. It intentionally does NOT duplicate Lumina metrics (which are already
// in Prometheus) - instead it focuses on what Veneer decided and what actions it took.
//
// Metrics are registered with controller-runtime's metrics registry, which automatically
// exposes them on the metrics endpoint (default :8080/metrics).
//
// This package follows Lumina's struct-based metrics pattern for consistency between
// the two projects. Metrics are encapsulated in a Metrics struct and initialized via
// NewMetrics(registry) factory function.
package metrics

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

// Metric namespace constant.
const (
	// Namespace is the Prometheus metric namespace for all Veneer metrics.
	Namespace = "veneer"
)

// Metric name constants.
const (
	MetricReconciliationDuration      = "reconciliation_duration_seconds"
	MetricReconciliationTotal         = "reconciliation_total"
	MetricLuminaDataFreshnessSeconds  = "lumina_data_freshness_seconds"
	MetricLuminaDataAvailable         = "lumina_data_available"
	MetricDecisionTotal               = "decision_total"
	MetricReservedInstanceDataAvail   = "reserved_instance_data_available"
	MetricReservedInstanceCount       = "reserved_instance_count"
	MetricOverlayOperationsTotal      = "overlay_operations_total"
	MetricOverlayOperationErrorsTotal = "overlay_operation_errors_total"
	MetricOverlayCount                = "overlay_count"
	MetricPrometheusQueryDuration     = "prometheus_query_duration_seconds"
	MetricPrometheusQueryErrorsTotal  = "prometheus_query_errors_total"
	MetricPrometheusQueryResultCount  = "prometheus_query_result_count"
	MetricConfigOverlaysDisabled      = "config_overlays_disabled"
	MetricConfigUtilizationThreshold  = "config_utilization_threshold_percent"
	MetricSPUtilizationPercent        = "savings_plan_utilization_percent"
	MetricSPRemainingCapacityDollars  = "savings_plan_remaining_capacity_dollars"
	MetricInfo                        = "info"
)

// Label key constants.
const (
	LabelResult         = "result"
	LabelOperation      = "operation"
	LabelErrorType      = "error_type"
	LabelQueryType      = "query_type"
	LabelCapacityType   = "capacity_type"
	LabelShouldExist    = "should_exist"
	LabelReason         = "reason"
	LabelInstanceType   = "instance_type"
	LabelInstanceFamily = "instance_family"
	LabelRegion         = "region"
	LabelType           = "type"
	LabelVersion        = "version"
	LabelDisabledMode   = "disabled_mode"
)

// Result represents the outcome of an operation.
type Result string

const (
	ResultSuccess Result = "success"
	ResultError   Result = "error"
)

// String returns the string representation of Result.
func (r Result) String() string {
	return string(r)
}

// Operation represents a NodeOverlay operation type.
type Operation string

const (
	OperationCreate Operation = "create"
	OperationUpdate Operation = "update"
	OperationDelete Operation = "delete"
)

// String returns the string representation of Operation.
func (o Operation) String() string {
	return string(o)
}

// ErrorType represents the type of error that occurred.
type ErrorType string

const (
	ErrorTypeValidation ErrorType = "validation"
	ErrorTypeAPI        ErrorType = "api"
	ErrorTypeNotFound   ErrorType = "not_found"
)

// String returns the string representation of ErrorType.
func (e ErrorType) String() string {
	return string(e)
}

// QueryType represents the type of Prometheus query.
type QueryType string

const (
	QueryTypeSPUtilization QueryType = "sp_utilization"
	QueryTypeSPCapacity    QueryType = "sp_capacity"
	QueryTypeRI            QueryType = "ri"
	QueryTypeDataFreshness QueryType = "data_freshness"
)

// String returns the string representation of QueryType.
func (q QueryType) String() string {
	return string(q)
}

// CapacityType represents the type of pre-paid AWS capacity.
type CapacityType string

const (
	CapacityTypeComputeSP     CapacityType = "compute_savings_plan"
	CapacityTypeEC2InstanceSP CapacityType = "ec2_instance_savings_plan"
	CapacityTypeRI            CapacityType = "reserved_instance"
)

// String returns the string representation of CapacityType.
func (c CapacityType) String() string {
	return string(c)
}

// DecisionReason represents the reason for a decision.
type DecisionReason string

const (
	ReasonCapacityAvailable         DecisionReason = "capacity_available"
	ReasonUtilizationAboveThreshold DecisionReason = "utilization_above_threshold"
	ReasonNoCapacity                DecisionReason = "no_capacity"
	ReasonRIAvailable               DecisionReason = "ri_available"
	ReasonRINotFound                DecisionReason = "ri_not_found"
	ReasonUnknown                   DecisionReason = "unknown"
)

// String returns the string representation of DecisionReason.
func (d DecisionReason) String() string {
	return string(d)
}

// ShouldExist represents whether an overlay should exist.
type ShouldExist string

const (
	ShouldExistTrue  ShouldExist = "true"
	ShouldExistFalse ShouldExist = "false"
)

// String returns the string representation of ShouldExist.
func (s ShouldExist) String() string {
	return string(s)
}

// Metric help text constants.
const (
	helpReconciliationDuration      = "Duration of metrics reconciliation cycles"
	helpReconciliationTotal         = "Total number of reconciliation cycles"
	helpLuminaDataFreshnessSeconds  = "Age of Lumina data in seconds"
	helpLuminaDataAvailable         = "1 if Lumina data is available and fresh, 0 if stale or unavailable"
	helpDecisionTotal               = "Total decisions made by capacity type and outcome"
	helpReservedInstanceDataAvail   = "1 if Lumina is exposing RI metrics, 0 if not"
	helpReservedInstanceCount       = "Number of Reserved Instances detected by instance type"
	helpOverlayOperationsTotal      = "Total NodeOverlay operations by type"
	helpOverlayOperationErrorsTotal = "Total NodeOverlay operation errors"
	helpOverlayCount                = "Current number of NodeOverlays managed by Veneer"
	helpPrometheusQueryDuration     = "Duration of Prometheus queries to Lumina metrics"
	helpPrometheusQueryErrorsTotal  = "Total Prometheus query errors"
	helpPrometheusQueryResultCount  = "Number of results returned by last Prometheus query"
	helpConfigOverlaysDisabled      = "1 if overlay creation is disabled (dry-run mode), 0 if enabled"
	helpConfigUtilizationThreshold  = "Configured utilization threshold for overlay deletion"
	helpSPUtilizationPercent        = "Savings Plan utilization percentage by type, family, and region"
	helpSPRemainingCapacityDollars  = "Savings Plan remaining capacity in dollars per hour"
	helpInfo                        = "Controller information with version and mode labels"
)

// Reason string patterns used for sanitization.
const (
	reasonPatternAboveThreshold = "at/above threshold"
	reasonPatternBelowThreshold = "below threshold"
	reasonPatternNoCapacity     = "no remaining capacity"
	reasonPatternRIAvailable    = "reserved instances available"
	reasonPatternNoRI           = "no reserved instances"
)

// Version is set at build time via ldflags.
var Version = "dev"

// Metrics holds all Prometheus metrics for the Veneer controller.
type Metrics struct {
	// ===================
	// Reconciliation Metrics
	// ===================

	// ReconciliationDuration tracks the duration of metrics reconciliation cycles.
	ReconciliationDuration prometheus.Histogram

	// ReconciliationTotal counts the total number of reconciliation cycles.
	ReconciliationTotal *prometheus.CounterVec

	// ===================
	// Data Source Health Metrics
	// ===================

	// LuminaDataFreshnessSeconds reports the age of Lumina data.
	LuminaDataFreshnessSeconds prometheus.Gauge

	// LuminaDataAvailable indicates whether Lumina data is available and fresh.
	LuminaDataAvailable prometheus.Gauge

	// ===================
	// Decision Metrics
	// ===================

	// DecisionTotal counts decisions made by the decision engine.
	DecisionTotal *prometheus.CounterVec

	// ===================
	// Reserved Instance Metrics
	// ===================

	// ReservedInstanceDataAvailable indicates whether Lumina is exposing RI data.
	ReservedInstanceDataAvailable prometheus.Gauge

	// ReservedInstanceCount tracks the number of RIs detected per instance type.
	ReservedInstanceCount *prometheus.GaugeVec

	// ===================
	// Savings Plan Metrics
	// ===================

	// SavingsPlanUtilizationPercent tracks SP utilization percentage.
	SavingsPlanUtilizationPercent *prometheus.GaugeVec

	// SavingsPlanRemainingCapacityDollars tracks remaining SP capacity in $/hour.
	SavingsPlanRemainingCapacityDollars *prometheus.GaugeVec

	// ===================
	// NodeOverlay Lifecycle Metrics
	// ===================

	// OverlayOperationsTotal counts NodeOverlay operations by type and capacity type.
	OverlayOperationsTotal *prometheus.CounterVec

	// OverlayOperationErrorsTotal counts NodeOverlay operation errors.
	OverlayOperationErrorsTotal *prometheus.CounterVec

	// OverlayCount tracks the current number of NodeOverlays managed by Veneer.
	OverlayCount *prometheus.GaugeVec

	// ===================
	// Prometheus Query Metrics
	// ===================

	// PrometheusQueryDuration tracks the duration of Prometheus queries to Lumina.
	PrometheusQueryDuration *prometheus.HistogramVec

	// PrometheusQueryErrorsTotal counts Prometheus query errors.
	PrometheusQueryErrorsTotal *prometheus.CounterVec

	// PrometheusQueryResultCount tracks the number of results returned by queries.
	PrometheusQueryResultCount *prometheus.GaugeVec

	// ===================
	// Configuration Metrics
	// ===================

	// ConfigOverlaysDisabled indicates whether overlay creation is disabled.
	ConfigOverlaysDisabled prometheus.Gauge

	// ConfigUtilizationThreshold reports the configured utilization threshold.
	ConfigUtilizationThreshold prometheus.Gauge

	// ===================
	// Info Metric
	// ===================

	// Info provides controller metadata as labels on a constant gauge.
	Info *prometheus.GaugeVec
}

// NewMetrics creates and registers all Prometheus metrics with the provided registry.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		ReconciliationDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: Namespace,
			Name:      MetricReconciliationDuration,
			Help:      helpReconciliationDuration,
			Buckets:   prometheus.ExponentialBuckets(0.1, 2, 10), // 0.1s to ~51s
		}),

		ReconciliationTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      MetricReconciliationTotal,
			Help:      helpReconciliationTotal,
		}, []string{LabelResult}),

		LuminaDataFreshnessSeconds: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      MetricLuminaDataFreshnessSeconds,
			Help:      helpLuminaDataFreshnessSeconds,
		}),

		LuminaDataAvailable: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      MetricLuminaDataAvailable,
			Help:      helpLuminaDataAvailable,
		}),

		DecisionTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      MetricDecisionTotal,
			Help:      helpDecisionTotal,
		}, []string{LabelCapacityType, LabelShouldExist, LabelReason}),

		ReservedInstanceDataAvailable: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      MetricReservedInstanceDataAvail,
			Help:      helpReservedInstanceDataAvail,
		}),

		ReservedInstanceCount: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      MetricReservedInstanceCount,
			Help:      helpReservedInstanceCount,
		}, []string{LabelInstanceType, LabelRegion}),

		SavingsPlanUtilizationPercent: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      MetricSPUtilizationPercent,
			Help:      helpSPUtilizationPercent,
		}, []string{LabelType, LabelInstanceFamily, LabelRegion}),

		SavingsPlanRemainingCapacityDollars: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      MetricSPRemainingCapacityDollars,
			Help:      helpSPRemainingCapacityDollars,
		}, []string{LabelType, LabelInstanceFamily, LabelRegion}),

		OverlayOperationsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      MetricOverlayOperationsTotal,
			Help:      helpOverlayOperationsTotal,
		}, []string{LabelOperation, LabelCapacityType}),

		OverlayOperationErrorsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      MetricOverlayOperationErrorsTotal,
			Help:      helpOverlayOperationErrorsTotal,
		}, []string{LabelOperation, LabelErrorType}),

		OverlayCount: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      MetricOverlayCount,
			Help:      helpOverlayCount,
		}, []string{LabelCapacityType}),

		PrometheusQueryDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: Namespace,
			Name:      MetricPrometheusQueryDuration,
			Help:      helpPrometheusQueryDuration,
			Buckets:   prometheus.DefBuckets,
		}, []string{LabelQueryType}),

		PrometheusQueryErrorsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      MetricPrometheusQueryErrorsTotal,
			Help:      helpPrometheusQueryErrorsTotal,
		}, []string{LabelQueryType}),

		PrometheusQueryResultCount: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      MetricPrometheusQueryResultCount,
			Help:      helpPrometheusQueryResultCount,
		}, []string{LabelQueryType}),

		ConfigOverlaysDisabled: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      MetricConfigOverlaysDisabled,
			Help:      helpConfigOverlaysDisabled,
		}),

		ConfigUtilizationThreshold: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      MetricConfigUtilizationThreshold,
			Help:      helpConfigUtilizationThreshold,
		}),

		Info: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      MetricInfo,
			Help:      helpInfo,
		}, []string{LabelVersion, LabelDisabledMode}),
	}

	// Register all metrics with the provided registry
	reg.MustRegister(
		m.ReconciliationDuration,
		m.ReconciliationTotal,
		m.LuminaDataFreshnessSeconds,
		m.LuminaDataAvailable,
		m.DecisionTotal,
		m.ReservedInstanceDataAvailable,
		m.ReservedInstanceCount,
		m.SavingsPlanUtilizationPercent,
		m.SavingsPlanRemainingCapacityDollars,
		m.OverlayOperationsTotal,
		m.OverlayOperationErrorsTotal,
		m.OverlayCount,
		m.PrometheusQueryDuration,
		m.PrometheusQueryErrorsTotal,
		m.PrometheusQueryResultCount,
		m.ConfigOverlaysDisabled,
		m.ConfigUtilizationThreshold,
		m.Info,
	)

	return m
}

// SetConfigMetrics sets configuration-related metrics. Call this once at startup.
func (m *Metrics) SetConfigMetrics(overlaysDisabled bool, utilizationThreshold float64) {
	if overlaysDisabled {
		m.ConfigOverlaysDisabled.Set(1)
	} else {
		m.ConfigOverlaysDisabled.Set(0)
	}
	m.ConfigUtilizationThreshold.Set(utilizationThreshold)

	// Set info metric
	disabledStr := "false"
	if overlaysDisabled {
		disabledStr = "true"
	}
	m.Info.WithLabelValues(Version, disabledStr).Set(1)
}

// RecordReconciliation records a reconciliation cycle result and duration.
func (m *Metrics) RecordReconciliation(result Result, durationSeconds float64) {
	m.ReconciliationTotal.WithLabelValues(result.String()).Inc()
	m.ReconciliationDuration.Observe(durationSeconds)
}

// RecordDecision records a decision made by the decision engine.
func (m *Metrics) RecordDecision(capacityType CapacityType, shouldExist ShouldExist, reason DecisionReason) {
	m.DecisionTotal.WithLabelValues(
		capacityType.String(),
		shouldExist.String(),
		reason.String(),
	).Inc()
}

// RecordOverlayOperation records a successful NodeOverlay operation.
func (m *Metrics) RecordOverlayOperation(operation Operation, capacityType CapacityType) {
	m.OverlayOperationsTotal.WithLabelValues(operation.String(), capacityType.String()).Inc()
}

// RecordOverlayOperationError records a failed NodeOverlay operation.
func (m *Metrics) RecordOverlayOperationError(operation Operation, errorType ErrorType) {
	m.OverlayOperationErrorsTotal.WithLabelValues(operation.String(), errorType.String()).Inc()
}

// RecordPrometheusQuery records a Prometheus query result.
func (m *Metrics) RecordPrometheusQuery(queryType QueryType, durationSeconds float64, resultCount int, err error) {
	m.PrometheusQueryDuration.WithLabelValues(queryType.String()).Observe(durationSeconds)
	m.PrometheusQueryResultCount.WithLabelValues(queryType.String()).Set(float64(resultCount))
	if err != nil {
		m.PrometheusQueryErrorsTotal.WithLabelValues(queryType.String()).Inc()
	}
}

// SetLuminaDataFreshness sets the Lumina data freshness metrics.
func (m *Metrics) SetLuminaDataFreshness(freshnessSeconds float64, maxFreshnessSeconds float64) {
	m.LuminaDataFreshnessSeconds.Set(freshnessSeconds)
	if freshnessSeconds < maxFreshnessSeconds {
		m.LuminaDataAvailable.Set(1)
	} else {
		m.LuminaDataAvailable.Set(0)
	}
}

// SetLuminaDataUnavailable marks Lumina data as unavailable.
func (m *Metrics) SetLuminaDataUnavailable() {
	m.LuminaDataAvailable.Set(0)
}

// SetReservedInstanceMetrics sets the RI-related metrics.
func (m *Metrics) SetReservedInstanceMetrics(dataAvailable bool, counts map[string]map[string]int) {
	if dataAvailable {
		m.ReservedInstanceDataAvailable.Set(1)
	} else {
		m.ReservedInstanceDataAvailable.Set(0)
	}

	for instanceType, regions := range counts {
		for region, count := range regions {
			m.ReservedInstanceCount.WithLabelValues(instanceType, region).Set(float64(count))
		}
	}
}

// SetSavingsPlanMetrics sets the SP utilization and remaining capacity metrics.
func (m *Metrics) SetSavingsPlanMetrics(
	spType string,
	instanceFamily string,
	region string,
	utilizationPercent float64,
	remainingCapacity float64,
) {
	// Use "all" for global compute SPs that aren't family-specific
	if instanceFamily == "" {
		instanceFamily = "all"
	}
	if region == "" {
		region = "global"
	}

	m.SavingsPlanUtilizationPercent.WithLabelValues(spType, instanceFamily, region).Set(utilizationPercent)
	m.SavingsPlanRemainingCapacityDollars.WithLabelValues(spType, instanceFamily, region).Set(remainingCapacity)
}

// SetOverlayCount sets the current overlay count by capacity type.
func (m *Metrics) SetOverlayCount(capacityType CapacityType, count int) {
	m.OverlayCount.WithLabelValues(capacityType.String()).Set(float64(count))
}

// BoolToShouldExist converts a boolean to a ShouldExist label value.
func BoolToShouldExist(b bool) ShouldExist {
	if b {
		return ShouldExistTrue
	}
	return ShouldExistFalse
}

// SanitizeReason converts a decision reason string to a controlled DecisionReason.
func SanitizeReason(reason string) DecisionReason {
	switch {
	case strings.Contains(reason, reasonPatternAboveThreshold):
		return ReasonUtilizationAboveThreshold
	case strings.Contains(reason, reasonPatternBelowThreshold):
		return ReasonCapacityAvailable
	case strings.Contains(reason, reasonPatternNoCapacity):
		return ReasonNoCapacity
	case strings.Contains(reason, reasonPatternNoRI):
		return ReasonRINotFound
	case strings.Contains(reason, reasonPatternRIAvailable):
		return ReasonRIAvailable
	default:
		return ReasonUnknown
	}
}

// CapacityTypeFromOverlay converts an overlay.CapacityType string to a metrics CapacityType.
func CapacityTypeFromOverlay(ct string) CapacityType {
	switch ct {
	case string(CapacityTypeComputeSP):
		return CapacityTypeComputeSP
	case string(CapacityTypeEC2InstanceSP):
		return CapacityTypeEC2InstanceSP
	case string(CapacityTypeRI):
		return CapacityTypeRI
	default:
		return CapacityType(ct)
	}
}
