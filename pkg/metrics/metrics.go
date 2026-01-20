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

// Package metrics provides Prometheus metrics for the Veneer controller.
// These metrics enable observability into reconciliation performance,
// overlay lifecycle, capacity utilization, and data quality.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	// Namespace is the Prometheus metric namespace for all Veneer metrics.
	Namespace = "veneer"

	// Label values for capacity types.
	CapacityTypeComputeSP     = "compute_savings_plan"
	CapacityTypeEC2InstanceSP = "ec2_instance_savings_plan"
	CapacityTypeRI            = "reserved_instance"

	// Label values for operations.
	OperationCreate = "create"
	OperationUpdate = "update"
	OperationDelete = "delete"

	// Label values for reconciliation status.
	StatusSuccess = "success"
	StatusError   = "error"
)

var (
	// ReconciliationDuration tracks the duration of each reconciliation cycle.
	// This histogram helps identify performance issues and set appropriate
	// reconciliation intervals.
	ReconciliationDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: Namespace,
		Name:      "reconciliation_duration_seconds",
		Help:      "Duration of reconciliation cycles in seconds",
		Buckets:   prometheus.ExponentialBuckets(0.1, 2, 10), // 0.1s to ~51s
	})

	// ReconciliationTotal counts the total number of reconciliation cycles
	// by their outcome status (success or error).
	ReconciliationTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: Namespace,
		Name:      "reconciliation_total",
		Help:      "Total number of reconciliation cycles by status",
	}, []string{"status"})

	// OverlayOperationsTotal counts overlay create/update/delete operations
	// by operation type and capacity type. This tracks overlay churn.
	OverlayOperationsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: Namespace,
		Name:      "overlay_operations_total",
		Help:      "Total overlay operations by operation type and capacity type",
	}, []string{"operation", "capacity_type"})

	// OverlayErrorsTotal counts errors during overlay operations.
	OverlayErrorsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: Namespace,
		Name:      "overlay_errors_total",
		Help:      "Total overlay operation errors by capacity type",
	}, []string{"capacity_type"})

	// OverlaysActive tracks the current number of active overlays by capacity type.
	// This gauge shows the current state of overlay management.
	OverlaysActive = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: Namespace,
		Name:      "overlays_active",
		Help:      "Current number of active overlays by capacity type",
	}, []string{"capacity_type"})

	// SavingsPlanUtilizationPercent tracks Savings Plan utilization percentage.
	// Labels identify the SP type, instance family (if applicable), and region.
	SavingsPlanUtilizationPercent = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: Namespace,
		Name:      "savings_plan_utilization_percent",
		Help:      "Savings Plan utilization percentage by type, family, and region",
	}, []string{"type", "instance_family", "region"})

	// SavingsPlanRemainingCapacityDollars tracks remaining SP capacity in $/hour.
	// This indicates how much pre-paid capacity is available for new workloads.
	SavingsPlanRemainingCapacityDollars = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: Namespace,
		Name:      "savings_plan_remaining_capacity_dollars",
		Help:      "Savings Plan remaining capacity in dollars per hour",
	}, []string{"type", "instance_family", "region"})

	// ReservedInstancesTotal tracks the count of Reserved Instances by type and region.
	ReservedInstancesTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: Namespace,
		Name:      "reserved_instances_total",
		Help:      "Total Reserved Instances by instance type and region",
	}, []string{"instance_type", "region"})

	// DataFreshnessSeconds tracks the age of Lumina data in seconds.
	// High values indicate stale data that may lead to incorrect decisions.
	DataFreshnessSeconds = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: Namespace,
		Name:      "data_freshness_seconds",
		Help:      "Age of Lumina capacity data in seconds",
	})

	// Info provides controller metadata as labels on a constant gauge.
	// This enables joining with other metrics to filter by version or mode.
	Info = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: Namespace,
		Name:      "info",
		Help:      "Controller information with version and mode labels",
	}, []string{"version", "disabled_mode"})

	// DecisionsTotal counts decisions made by the decision engine.
	// Labels indicate whether the decision was to create or skip an overlay.
	DecisionsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: Namespace,
		Name:      "decisions_total",
		Help:      "Total decisions made by capacity type and outcome",
	}, []string{"capacity_type", "should_exist"})
)

func init() {
	// Register all metrics with the controller-runtime metrics registry.
	// This registry is automatically exposed on the metrics endpoint.
	metrics.Registry.MustRegister(
		ReconciliationDuration,
		ReconciliationTotal,
		OverlayOperationsTotal,
		OverlayErrorsTotal,
		OverlaysActive,
		SavingsPlanUtilizationPercent,
		SavingsPlanRemainingCapacityDollars,
		ReservedInstancesTotal,
		DataFreshnessSeconds,
		Info,
		DecisionsTotal,
	)
}
