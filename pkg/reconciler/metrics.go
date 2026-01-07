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

// Package reconciler provides Kubernetes controllers for managing cost-aware provisioning.
//
// The metrics reconciler periodically queries Prometheus for Lumina metrics and logs
// the current state of Savings Plans and Reserved Instances capacity.
package reconciler

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/nextdoor/veneer/pkg/prometheus"
)

// MetricsReconciler periodically queries Prometheus for Lumina metrics.
// It logs the current state of Savings Plans and Reserved Instances capacity
// to provide visibility into cost optimization data.
type MetricsReconciler struct {
	// PrometheusClient is the client for querying Lumina metrics
	PrometheusClient *prometheus.Client

	// Logger is the structured logger for this reconciler
	Logger logr.Logger

	// Interval is how often to query metrics (default: 5 minutes)
	Interval time.Duration
}

// Start begins the metrics reconciliation loop.
// It runs until the context is cancelled.
func (r *MetricsReconciler) Start(ctx context.Context) error {
	r.Logger.Info("Starting metrics reconciler", "interval", r.Interval)

	// Use default interval if not set
	if r.Interval == 0 {
		r.Interval = 5 * time.Minute
	}

	ticker := time.NewTicker(r.Interval)
	defer ticker.Stop()

	// Run once immediately on startup
	if err := r.reconcile(ctx); err != nil {
		r.Logger.Error(err, "Failed to reconcile metrics on startup")
		// Don't fail startup on first reconcile error
	}

	// Then run on the ticker interval
	for {
		select {
		case <-ctx.Done():
			r.Logger.Info("Metrics reconciler stopped")
			return nil
		case <-ticker.C:
			if err := r.reconcile(ctx); err != nil {
				r.Logger.Error(err, "Failed to reconcile metrics")
				// Continue running even on error
			}
		}
	}
}

// reconcile queries Prometheus and logs current metrics state.
func (r *MetricsReconciler) reconcile(ctx context.Context) error {
	r.Logger.V(1).Info("Reconciling metrics")

	// Check data freshness
	freshness, err := r.PrometheusClient.DataFreshness(ctx)
	if err != nil {
		return fmt.Errorf("failed to query data freshness: %w", err)
	}
	r.Logger.Info("Lumina data freshness", "age_seconds", freshness)

	// Query Savings Plans capacity (all families)
	spCapacities, err := r.PrometheusClient.QuerySavingsPlanCapacity(ctx, "")
	if err != nil {
		return fmt.Errorf("failed to query Savings Plans capacity: %w", err)
	}

	// Log SP capacity by instance family
	for _, sp := range spCapacities {
		r.Logger.Info("Savings Plan capacity",
			"instance_family", sp.InstanceFamily,
			"type", sp.Type,
			"remaining_capacity_dollars_per_hour", sp.RemainingCapacity,
			"savings_plan_arn", sp.SavingsPlanARN,
		)
	}

	// Query Reserved Instances (all types)
	ris, err := r.PrometheusClient.QueryReservedInstances(ctx, "")
	if err != nil {
		return fmt.Errorf("failed to query Reserved Instances: %w", err)
	}

	// Log RI count by instance type
	riCounts := make(map[string]int)
	for _, ri := range ris {
		riCounts[ri.InstanceType] += ri.Count
	}

	for instanceType, count := range riCounts {
		r.Logger.Info("Reserved Instance availability",
			"instance_type", instanceType,
			"count", count,
		)
	}

	r.Logger.V(1).Info("Metrics reconciliation complete",
		"savings_plans_count", len(spCapacities),
		"reserved_instances_count", len(ris),
	)

	return nil
}
