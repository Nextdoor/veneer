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
package metrics

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// Metric namespace and subsystem constants.
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
	MetricBuildInfo                   = "build_info"
)

// Label key constants.
const (
	LabelResult       = "result"
	LabelOperation    = "operation"
	LabelErrorType    = "error_type"
	LabelQueryType    = "query_type"
	LabelCapacityType = "capacity_type"
	LabelShouldExist  = "should_exist"
	LabelReason       = "reason"
	LabelInstanceType = "instance_type"
	LabelRegion       = "region"
	LabelVersion      = "version"
	LabelCommit       = "commit"
	LabelBuildDate    = "build_date"
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

// FromBool converts a boolean to ShouldExist.
func (s ShouldExist) FromBool(b bool) ShouldExist {
	if b {
		return ShouldExistTrue
	}
	return ShouldExistFalse
}

// Metric help text constants.
const (
	helpReconciliationDuration      = "Duration of metrics reconciliation cycles"
	helpReconciliationTotal         = "Total number of reconciliation cycles"
	helpLuminaDataFreshnessSeconds  = "Age of Lumina data in seconds (from lumina_data_freshness_seconds metric)"
	helpLuminaDataAvailable         = "1 if Lumina data is available and fresh, 0 if stale or unavailable"
	helpDecisionTotal               = "Total decisions made by capacity type and outcome"
	helpReservedInstanceDataAvail   = "1 if Lumina is exposing RI metrics (ec2_reserved_instance), 0 if not"
	helpReservedInstanceCount       = "Number of Reserved Instances detected by instance type"
	helpOverlayOperationsTotal      = "Total NodeOverlay operations by type"
	helpOverlayOperationErrorsTotal = "Total NodeOverlay operation errors"
	helpOverlayCount                = "Current number of NodeOverlays managed by Veneer"
	helpPrometheusQueryDuration     = "Duration of Prometheus queries to Lumina metrics"
	helpPrometheusQueryErrorsTotal  = "Total Prometheus query errors"
	helpPrometheusQueryResultCount  = "Number of results returned by last Prometheus query (0 may indicate missing data)"
	helpConfigOverlaysDisabled      = "1 if overlay creation is disabled (dry-run mode), 0 if enabled"
	helpConfigUtilizationThreshold  = "Configured utilization threshold for overlay deletion (default 95%)"
	helpBuildInfo                   = "Build information for Veneer"
)

// Reason string patterns used for sanitization.
// These are substrings that appear in decision reason messages.
const (
	reasonPatternAboveThreshold = "at/above threshold"
	reasonPatternBelowThreshold = "below threshold"
	reasonPatternNoCapacity     = "no remaining capacity"
	reasonPatternRIAvailable    = "reserved instances available"
	reasonPatternNoRI           = "no reserved instances"
)

var (
	// ===================
	// Reconciliation Metrics
	// ===================

	// ReconciliationDuration tracks the duration of metrics reconciliation cycles.
	// This helps identify performance issues and establish baseline latency.
	ReconciliationDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: Namespace,
		Name:      MetricReconciliationDuration,
		Help:      helpReconciliationDuration,
		Buckets:   prometheus.DefBuckets,
	})

	// ReconciliationTotal counts the total number of reconciliation cycles.
	// Labels: result (success, error)
	ReconciliationTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: Namespace,
		Name:      MetricReconciliationTotal,
		Help:      helpReconciliationTotal,
	}, []string{LabelResult})

	// ===================
	// Data Source Health Metrics
	// ===================

	// LuminaDataFreshnessSeconds reports the age of Lumina data.
	// This is derived from the lumina_data_freshness_seconds metric and helps
	// identify when Veneer is operating on stale data.
	LuminaDataFreshnessSeconds = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: Namespace,
		Name:      MetricLuminaDataFreshnessSeconds,
		Help:      helpLuminaDataFreshnessSeconds,
	})

	// LuminaDataAvailable indicates whether Lumina data is available and fresh.
	// 1 = data available and fresh (< 10 minutes old), 0 = stale or unavailable.
	// This is useful for alerting on data availability issues.
	LuminaDataAvailable = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: Namespace,
		Name:      MetricLuminaDataAvailable,
		Help:      helpLuminaDataAvailable,
	})

	// ===================
	// Decision Metrics
	// ===================

	// DecisionTotal counts decisions made by the decision engine.
	// Labels: capacity_type, should_exist, reason
	// This is the most important metric for understanding Veneer's behavior.
	DecisionTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: Namespace,
		Name:      MetricDecisionTotal,
		Help:      helpDecisionTotal,
	}, []string{LabelCapacityType, LabelShouldExist, LabelReason})

	// ===================
	// Reserved Instance Metrics
	// ===================

	// ReservedInstanceDataAvailable indicates whether Lumina is exposing RI data.
	// NOTE: Lumina does not currently expose ec2_reserved_instance metric.
	// This metric helps distinguish "no RIs exist" from "Lumina not exporting RI data".
	ReservedInstanceDataAvailable = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: Namespace,
		Name:      MetricReservedInstanceDataAvail,
		Help:      helpReservedInstanceDataAvail,
	})

	// ReservedInstanceCount tracks the number of RIs detected per instance type.
	// Labels: instance_type, region
	// This will be populated when Lumina starts exposing RI data.
	ReservedInstanceCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: Namespace,
		Name:      MetricReservedInstanceCount,
		Help:      helpReservedInstanceCount,
	}, []string{LabelInstanceType, LabelRegion})

	// ===================
	// NodeOverlay Lifecycle Metrics
	// ===================

	// OverlayOperationsTotal counts NodeOverlay operations by type and capacity type.
	// Labels: operation (create, update, delete), capacity_type
	OverlayOperationsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: Namespace,
		Name:      MetricOverlayOperationsTotal,
		Help:      helpOverlayOperationsTotal,
	}, []string{LabelOperation, LabelCapacityType})

	// OverlayOperationErrorsTotal counts NodeOverlay operation errors.
	// Labels: operation (create, update, delete), error_type (validation, api, not_found)
	OverlayOperationErrorsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: Namespace,
		Name:      MetricOverlayOperationErrorsTotal,
		Help:      helpOverlayOperationErrorsTotal,
	}, []string{LabelOperation, LabelErrorType})

	// OverlayCount tracks the current number of NodeOverlays managed by Veneer.
	// Labels: capacity_type
	// This is a gauge that gets updated after each reconciliation cycle.
	OverlayCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: Namespace,
		Name:      MetricOverlayCount,
		Help:      helpOverlayCount,
	}, []string{LabelCapacityType})

	// ===================
	// Prometheus Query Metrics
	// ===================

	// PrometheusQueryDuration tracks the duration of Prometheus queries to Lumina.
	// Labels: query_type (sp_utilization, sp_capacity, ri, data_freshness)
	PrometheusQueryDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: Namespace,
		Name:      MetricPrometheusQueryDuration,
		Help:      helpPrometheusQueryDuration,
		Buckets:   prometheus.DefBuckets,
	}, []string{LabelQueryType})

	// PrometheusQueryErrorsTotal counts Prometheus query errors.
	// Labels: query_type
	PrometheusQueryErrorsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: Namespace,
		Name:      MetricPrometheusQueryErrorsTotal,
		Help:      helpPrometheusQueryErrorsTotal,
	}, []string{LabelQueryType})

	// PrometheusQueryResultCount tracks the number of results returned by queries.
	// Labels: query_type
	// A value of 0 may indicate missing data in Prometheus.
	PrometheusQueryResultCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: Namespace,
		Name:      MetricPrometheusQueryResultCount,
		Help:      helpPrometheusQueryResultCount,
	}, []string{LabelQueryType})

	// ===================
	// Configuration Metrics
	// ===================

	// ConfigOverlaysDisabled indicates whether overlay creation is disabled.
	// 1 = disabled (dry-run mode), 0 = enabled.
	ConfigOverlaysDisabled = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: Namespace,
		Name:      MetricConfigOverlaysDisabled,
		Help:      helpConfigOverlaysDisabled,
	})

	// ConfigUtilizationThreshold reports the configured utilization threshold.
	// Default is 95% - overlays are deleted when utilization reaches this threshold.
	ConfigUtilizationThreshold = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: Namespace,
		Name:      MetricConfigUtilizationThreshold,
		Help:      helpConfigUtilizationThreshold,
	})

	// ===================
	// Info Metric
	// ===================

	// BuildInfo provides build information as a metric.
	// Labels: version, commit, build_date
	// The value is always 1; the information is in the labels.
	BuildInfo = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: Namespace,
		Name:      MetricBuildInfo,
		Help:      helpBuildInfo,
	}, []string{LabelVersion, LabelCommit, LabelBuildDate})
)

func init() {
	// Register all metrics with controller-runtime's registry.
	// This makes them available on the metrics endpoint automatically.
	metrics.Registry.MustRegister(
		// Reconciliation
		ReconciliationDuration,
		ReconciliationTotal,
		// Data source health
		LuminaDataFreshnessSeconds,
		LuminaDataAvailable,
		// Decisions
		DecisionTotal,
		// Reserved Instances
		ReservedInstanceDataAvailable,
		ReservedInstanceCount,
		// NodeOverlay lifecycle
		OverlayOperationsTotal,
		OverlayOperationErrorsTotal,
		OverlayCount,
		// Prometheus queries
		PrometheusQueryDuration,
		PrometheusQueryErrorsTotal,
		PrometheusQueryResultCount,
		// Configuration
		ConfigOverlaysDisabled,
		ConfigUtilizationThreshold,
		// Info
		BuildInfo,
	)
}

// SetBuildInfo sets the build info metric. Call this once at startup.
func SetBuildInfo(version, commit, buildDate string) {
	BuildInfo.WithLabelValues(version, commit, buildDate).Set(1)
}

// SetConfigMetrics sets configuration-related metrics. Call this once at startup
// and whenever configuration changes.
func SetConfigMetrics(overlaysDisabled bool, utilizationThreshold float64) {
	if overlaysDisabled {
		ConfigOverlaysDisabled.Set(1)
	} else {
		ConfigOverlaysDisabled.Set(0)
	}
	ConfigUtilizationThreshold.Set(utilizationThreshold)
}

// BoolToShouldExist converts a boolean to a ShouldExist label value.
func BoolToShouldExist(b bool) ShouldExist {
	if b {
		return ShouldExistTrue
	}
	return ShouldExistFalse
}

// SanitizeReason converts a decision reason string to a controlled DecisionReason.
// This prevents high cardinality from dynamic reason strings while preserving
// the essential information about why a decision was made.
//
// Note: The order of checks matters - more specific patterns must come before
// patterns that are substrings of longer patterns (e.g., "no reserved instances"
// must be checked before "reserved instances available").
func SanitizeReason(reason string) DecisionReason {
	switch {
	case strings.Contains(reason, reasonPatternAboveThreshold):
		return ReasonUtilizationAboveThreshold
	case strings.Contains(reason, reasonPatternBelowThreshold):
		return ReasonCapacityAvailable
	case strings.Contains(reason, reasonPatternNoCapacity):
		return ReasonNoCapacity
	case strings.Contains(reason, reasonPatternNoRI):
		// Must check "no reserved instances" before "reserved instances available"
		// because the latter is a substring of the former
		return ReasonRINotFound
	case strings.Contains(reason, reasonPatternRIAvailable):
		return ReasonRIAvailable
	default:
		return ReasonUnknown
	}
}

// CapacityTypeFromOverlay converts an overlay.CapacityType string to a metrics CapacityType.
// This ensures consistent labeling across all metrics.
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

// RecordReconciliation records a reconciliation cycle result and duration.
func RecordReconciliation(result Result, durationSeconds float64) {
	ReconciliationTotal.WithLabelValues(result.String()).Inc()
	ReconciliationDuration.Observe(durationSeconds)
}

// RecordDecision records a decision made by the decision engine.
func RecordDecision(capacityType CapacityType, shouldExist ShouldExist, reason DecisionReason) {
	DecisionTotal.WithLabelValues(
		capacityType.String(),
		shouldExist.String(),
		reason.String(),
	).Inc()
}

// RecordOverlayOperation records a successful NodeOverlay operation.
func RecordOverlayOperation(operation Operation, capacityType CapacityType) {
	OverlayOperationsTotal.WithLabelValues(operation.String(), capacityType.String()).Inc()
}

// RecordOverlayOperationError records a failed NodeOverlay operation.
func RecordOverlayOperationError(operation Operation, errorType ErrorType) {
	OverlayOperationErrorsTotal.WithLabelValues(operation.String(), errorType.String()).Inc()
}

// RecordPrometheusQuery records a Prometheus query result.
func RecordPrometheusQuery(queryType QueryType, durationSeconds float64, resultCount int, err error) {
	PrometheusQueryDuration.WithLabelValues(queryType.String()).Observe(durationSeconds)
	PrometheusQueryResultCount.WithLabelValues(queryType.String()).Set(float64(resultCount))
	if err != nil {
		PrometheusQueryErrorsTotal.WithLabelValues(queryType.String()).Inc()
	}
}

// SetLuminaDataFreshness sets the Lumina data freshness metrics.
// freshnessSeconds is the age of the data in seconds.
// Data is considered "available" if freshnessSeconds < maxFreshnessSeconds.
func SetLuminaDataFreshness(freshnessSeconds float64, maxFreshnessSeconds float64) {
	LuminaDataFreshnessSeconds.Set(freshnessSeconds)
	if freshnessSeconds < maxFreshnessSeconds {
		LuminaDataAvailable.Set(1)
	} else {
		LuminaDataAvailable.Set(0)
	}
}

// SetLuminaDataUnavailable marks Lumina data as unavailable.
func SetLuminaDataUnavailable() {
	LuminaDataAvailable.Set(0)
}

// SetReservedInstanceMetrics sets the RI-related metrics.
func SetReservedInstanceMetrics(dataAvailable bool, counts map[string]map[string]int) {
	if dataAvailable {
		ReservedInstanceDataAvailable.Set(1)
	} else {
		ReservedInstanceDataAvailable.Set(0)
	}

	for instanceType, regions := range counts {
		for region, count := range regions {
			ReservedInstanceCount.WithLabelValues(instanceType, region).Set(float64(count))
		}
	}
}

// SetOverlayCount sets the current overlay count by capacity type.
func SetOverlayCount(capacityType CapacityType, count int) {
	OverlayCount.WithLabelValues(capacityType.String()).Set(float64(count))
}
