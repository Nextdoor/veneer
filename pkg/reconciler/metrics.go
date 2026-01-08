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
// The metrics reconciler periodically queries Prometheus for Lumina metrics,
// makes overlay lifecycle decisions, and creates/updates/deletes NodeOverlay resources.
package reconciler

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/nextdoor/veneer/pkg/config"
	"github.com/nextdoor/veneer/pkg/overlay"
	"github.com/nextdoor/veneer/pkg/prometheus"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	karpenterv1alpha1 "sigs.k8s.io/karpenter/pkg/apis/v1alpha1"
)

// MetricsReconciler periodically queries Prometheus for Lumina metrics.
// It analyzes capacity utilization, makes overlay lifecycle decisions,
// and creates/updates/deletes NodeOverlay resources in the cluster.
type MetricsReconciler struct {
	// PrometheusClient is the client for querying Lumina metrics
	PrometheusClient *prometheus.Client

	// Config is the controller configuration
	Config *config.Config

	// DecisionEngine analyzes capacity and determines overlay lifecycle
	DecisionEngine *overlay.DecisionEngine

	// Generator creates NodeOverlay specs from decisions
	Generator *overlay.Generator

	// Client is the Kubernetes client for managing NodeOverlay resources
	Client client.Client

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

// reconcile queries Prometheus, makes overlay decisions, and generates NodeOverlay specs.
func (r *MetricsReconciler) reconcile(ctx context.Context) error {
	r.Logger.V(1).Info("Reconciling metrics")

	// Check data freshness
	freshness, err := r.PrometheusClient.DataFreshness(ctx)
	if err != nil {
		return fmt.Errorf("failed to query data freshness: %w", err)
	}
	r.Logger.Info("Lumina data freshness", "age_seconds", freshness)

	// Collect all decisions
	var decisions []overlay.Decision

	// Query and analyze Compute Savings Plans
	computeDecisions, err := r.analyzeComputeSavingsPlans(ctx)
	if err != nil {
		r.Logger.Error(err, "Failed to analyze Compute Savings Plans")
	} else {
		decisions = append(decisions, computeDecisions...)
	}

	// Query and analyze EC2 Instance Savings Plans
	ec2Decisions, err := r.analyzeEC2InstanceSavingsPlans(ctx)
	if err != nil {
		r.Logger.Error(err, "Failed to analyze EC2 Instance Savings Plans")
	} else {
		decisions = append(decisions, ec2Decisions...)
	}

	// Query and analyze Reserved Instances
	riDecisions, err := r.analyzeReservedInstances(ctx)
	if err != nil {
		r.Logger.Error(err, "Failed to analyze Reserved Instances")
	} else {
		decisions = append(decisions, riDecisions...)
	}

	// Generate and apply NodeOverlay specs from decisions
	if r.Generator != nil && r.Client != nil && len(decisions) > 0 {
		generatedOverlays := r.Generator.GenerateAll(decisions)
		r.applyOverlays(ctx, generatedOverlays)
	}

	r.Logger.V(1).Info("Metrics reconciliation complete",
		"decisions_count", len(decisions),
	)

	return nil
}

// analyzeComputeSavingsPlans queries and analyzes Compute Savings Plans.
func (r *MetricsReconciler) analyzeComputeSavingsPlans(ctx context.Context) ([]overlay.Decision, error) {
	// Query utilization
	utilizations, err := r.PrometheusClient.QuerySavingsPlanUtilization(ctx, prometheus.SavingsPlanTypeCompute)
	if err != nil {
		return nil, fmt.Errorf("failed to query Compute SP utilization: %w", err)
	}

	// Query capacity
	capacities, err := r.PrometheusClient.QuerySavingsPlanCapacity(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("failed to query SP capacity: %w", err)
	}

	// Filter to just Compute SPs
	var computeCapacities []prometheus.SavingsPlanCapacity
	for _, cap := range capacities {
		if cap.Type == prometheus.SavingsPlanTypeCompute {
			computeCapacities = append(computeCapacities, cap)
		}
	}

	if len(computeCapacities) == 0 {
		r.Logger.V(1).Info("No Compute Savings Plans found")
		return nil, nil
	}

	// Aggregate and analyze
	agg := overlay.AggregateComputeSavingsPlans(utilizations, computeCapacities)
	if r.DecisionEngine == nil {
		return nil, nil
	}

	decision := r.DecisionEngine.AnalyzeComputeSavingsPlan(agg)

	r.Logger.Info("Compute Savings Plan analysis",
		"total_remaining_capacity", agg.TotalRemainingCapacity,
		"utilization_percent", agg.UtilizationPercent,
		"should_exist", decision.ShouldExist,
		"reason", decision.Reason,
	)

	return []overlay.Decision{decision}, nil
}

// analyzeEC2InstanceSavingsPlans queries and analyzes EC2 Instance Savings Plans.
func (r *MetricsReconciler) analyzeEC2InstanceSavingsPlans(ctx context.Context) ([]overlay.Decision, error) {
	// Query utilization
	utilizations, err := r.PrometheusClient.QuerySavingsPlanUtilization(ctx, prometheus.SavingsPlanTypeEC2Instance)
	if err != nil {
		return nil, fmt.Errorf("failed to query EC2 Instance SP utilization: %w", err)
	}

	// Query capacity for all families
	capacities, err := r.PrometheusClient.QuerySavingsPlanCapacity(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("failed to query SP capacity: %w", err)
	}

	// Filter to just EC2 Instance SPs
	var ec2Capacities []prometheus.SavingsPlanCapacity
	for _, cap := range capacities {
		if cap.Type == prometheus.SavingsPlanTypeEC2Instance {
			ec2Capacities = append(ec2Capacities, cap)
		}
	}

	if len(ec2Capacities) == 0 {
		r.Logger.V(1).Info("No EC2 Instance Savings Plans found")
		return nil, nil
	}

	// Aggregate by family+region and analyze each
	aggByFamily := overlay.AggregateEC2InstanceSavingsPlans(utilizations, ec2Capacities)
	if r.DecisionEngine == nil {
		return nil, nil
	}

	var decisions []overlay.Decision
	for key, agg := range aggByFamily {
		decision := r.DecisionEngine.AnalyzeEC2InstanceSavingsPlan(agg)

		r.Logger.Info("EC2 Instance Savings Plan analysis",
			"family_region", key,
			"total_remaining_capacity", agg.TotalRemainingCapacity,
			"utilization_percent", agg.UtilizationPercent,
			"should_exist", decision.ShouldExist,
			"reason", decision.Reason,
		)

		decisions = append(decisions, decision)
	}

	return decisions, nil
}

// analyzeReservedInstances queries and analyzes Reserved Instances.
func (r *MetricsReconciler) analyzeReservedInstances(ctx context.Context) ([]overlay.Decision, error) {
	// Query all RIs
	ris, err := r.PrometheusClient.QueryReservedInstances(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("failed to query Reserved Instances: %w", err)
	}

	if len(ris) == 0 {
		r.Logger.V(1).Info("No Reserved Instances found")
		return nil, nil
	}

	// Aggregate by instance type+region and analyze each
	aggByType := overlay.AggregateReservedInstances(ris)
	if r.DecisionEngine == nil {
		return nil, nil
	}

	var decisions []overlay.Decision
	for key, agg := range aggByType {
		decision := r.DecisionEngine.AnalyzeReservedInstance(agg)

		r.Logger.Info("Reserved Instance analysis",
			"type_region", key,
			"total_count", agg.TotalCount,
			"should_exist", decision.ShouldExist,
			"reason", decision.Reason,
		)

		decisions = append(decisions, decision)
	}

	return decisions, nil
}

// applyOverlays creates, updates, or deletes NodeOverlay resources based on decisions.
func (r *MetricsReconciler) applyOverlays(ctx context.Context, overlays []overlay.GeneratedOverlay) {
	createCount := 0
	updateCount := 0
	deleteCount := 0
	errorCount := 0

	for _, gen := range overlays {
		switch gen.Action {
		case "create":
			if gen.Overlay != nil {
				// Validate the generated overlay
				if validationErrors := overlay.ValidateOverlay(gen.Overlay); len(validationErrors) > 0 {
					r.Logger.Error(nil, "Generated overlay failed validation",
						"name", gen.Overlay.Name,
						"errors", validationErrors,
					)
					errorCount++
					continue
				}

				// Check if overlay already exists
				existing := &karpenterv1alpha1.NodeOverlay{}
				err := r.Client.Get(ctx, client.ObjectKey{Name: gen.Overlay.Name}, existing)

				if errors.IsNotFound(err) {
					// Create new overlay
					if err := r.Client.Create(ctx, gen.Overlay); err != nil {
						r.Logger.Error(err, "Failed to create NodeOverlay",
							"name", gen.Overlay.Name,
						)
						errorCount++
						continue
					}
					createCount++
					r.Logger.Info("Created NodeOverlay",
						"name", gen.Overlay.Name,
						"capacity_type", gen.Decision.CapacityType,
						"weight", gen.Decision.Weight,
						"price", gen.Decision.Price,
						"reason", gen.Decision.Reason,
					)
				} else if err != nil {
					r.Logger.Error(err, "Failed to check existing NodeOverlay",
						"name", gen.Overlay.Name,
					)
					errorCount++
					continue
				} else {
					// Update existing overlay if spec differs
					// Copy the resource version from existing to allow update
					gen.Overlay.ResourceVersion = existing.ResourceVersion
					if err := r.Client.Update(ctx, gen.Overlay); err != nil {
						r.Logger.Error(err, "Failed to update NodeOverlay",
							"name", gen.Overlay.Name,
						)
						errorCount++
						continue
					}
					updateCount++
					r.Logger.V(1).Info("Updated NodeOverlay",
						"name", gen.Overlay.Name,
						"capacity_type", gen.Decision.CapacityType,
					)
				}
			}

		case "delete":
			// Check if overlay exists before deleting
			existing := &karpenterv1alpha1.NodeOverlay{}
			err := r.Client.Get(ctx, client.ObjectKey{Name: gen.Decision.Name}, existing)

			if errors.IsNotFound(err) {
				// Already gone, nothing to do
				r.Logger.V(1).Info("NodeOverlay already deleted",
					"name", gen.Decision.Name,
				)
				continue
			} else if err != nil {
				r.Logger.Error(err, "Failed to check existing NodeOverlay for deletion",
					"name", gen.Decision.Name,
				)
				errorCount++
				continue
			}

			// Only delete if it's managed by Veneer
			if existing.Labels[overlay.LabelManagedBy] != overlay.LabelManagedByValue {
				r.Logger.Info("Skipping deletion of NodeOverlay not managed by Veneer",
					"name", gen.Decision.Name,
					"managed_by", existing.Labels[overlay.LabelManagedBy],
				)
				continue
			}

			if err := r.Client.Delete(ctx, existing); err != nil {
				r.Logger.Error(err, "Failed to delete NodeOverlay",
					"name", gen.Decision.Name,
				)
				errorCount++
				continue
			}
			deleteCount++
			r.Logger.Info("Deleted NodeOverlay",
				"name", gen.Decision.Name,
				"capacity_type", gen.Decision.CapacityType,
				"reason", gen.Decision.Reason,
			)
		}
	}

	r.Logger.Info("NodeOverlay reconciliation summary",
		"created", createCount,
		"updated", updateCount,
		"deleted", deleteCount,
		"errors", errorCount,
	)
}
