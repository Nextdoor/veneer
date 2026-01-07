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

// Package overlay provides decision logic for NodeOverlay lifecycle management.
//
// This package determines when to create, update, or delete Karpenter NodeOverlay CRs
// based on Savings Plan and Reserved Instance capacity/utilization metrics from Lumina.
package overlay

import (
	"fmt"

	"github.com/nextdoor/veneer/pkg/config"
	"github.com/nextdoor/veneer/pkg/prometheus"
)

// CapacityType represents the type of pre-paid AWS capacity backing an overlay.
type CapacityType string

const (
	// CapacityTypeComputeSavingsPlan represents a global Compute Savings Plan (all families, all regions).
	CapacityTypeComputeSavingsPlan CapacityType = "compute_savings_plan"

	// CapacityTypeEC2InstanceSavingsPlan represents a family-specific EC2 Instance Savings Plan.
	CapacityTypeEC2InstanceSavingsPlan CapacityType = "ec2_instance_savings_plan"

	// CapacityTypeReservedInstance represents an instance-type-specific Reserved Instance.
	CapacityTypeReservedInstance CapacityType = "reserved_instance"
)

// Decision represents whether a NodeOverlay should exist for a specific capacity source.
//
// The decision engine analyzes capacity utilization and determines if Karpenter should
// prefer on-demand instances (via overlay) or let spot remain the default choice.
type Decision struct {
	// Name is the unique overlay name (e.g., "cost-aware-compute-sp-global", "cost-aware-ri-m5-xlarge").
	Name string

	// CapacityType identifies what type of pre-paid capacity this overlay represents.
	CapacityType CapacityType

	// ShouldExist indicates whether the overlay should be created/kept (true) or deleted (false).
	// True = utilization below threshold AND remaining capacity available
	// False = utilization at/above threshold OR no remaining capacity
	ShouldExist bool

	// Weight is the Karpenter overlay precedence (higher = higher priority).
	// Reserved Instances > EC2 Instance SPs > Compute SPs
	Weight int

	// Price is the effective hourly cost for on-demand instances with this capacity applied.
	// For Phase 2, this is always "0.00" (100% discount) to maximize pre-paid usage.
	Price string

	// TargetSelector describes which instances this overlay targets.
	// Examples:
	//   - Global Compute SP: "karpenter.k8s.aws/instance-family: Exists"
	//   - EC2 Instance SP (m5): "karpenter.k8s.aws/instance-family: In [m5]"
	//   - RI (m5.xlarge): "node.kubernetes.io/instance-type: In [m5.xlarge]"
	TargetSelector string

	// Reason explains why this decision was made (for logging/debugging).
	// Examples: "utilization 87% below threshold 95%", "no remaining capacity", "capacity available"
	Reason string

	// UtilizationPercent is the current utilization percentage of the backing capacity (0-100+).
	// Optional: may be 0 for RIs (which don't have utilization metrics).
	UtilizationPercent float64

	// RemainingCapacity is the remaining capacity in $/hour.
	// Optional: may be 0 if not applicable or unknown.
	RemainingCapacity float64
}

// DecisionEngine analyzes capacity metrics and produces overlay lifecycle decisions.
type DecisionEngine struct {
	// Config provides utilization thresholds and overlay weights.
	Config *config.Config
}

// NewDecisionEngine creates a new decision engine with the provided configuration.
func NewDecisionEngine(cfg *config.Config) *DecisionEngine {
	return &DecisionEngine{
		Config: cfg,
	}
}

// AggregatedSavingsPlan represents aggregated metrics for multiple Savings Plans of the same type/family.
type AggregatedSavingsPlan struct {
	// Type is the SP type ("compute" or "ec2_instance")
	Type string

	// InstanceFamily is the instance family (only for EC2 Instance SPs, empty for Compute SPs)
	InstanceFamily string

	// Region is the region (only for EC2 Instance SPs, empty for Compute SPs)
	Region string

	// AccountID is the AWS account ID (from first SP in group)
	AccountID string

	// TotalRemainingCapacity is the sum of all remaining capacities in $/hour
	TotalRemainingCapacity float64

	// TotalHourlyCommitment is the sum of all hourly commitments in $/hour
	TotalHourlyCommitment float64

	// UtilizationPercent is calculated as (1 - remaining/commitment) * 100
	UtilizationPercent float64

	// Count is the number of SPs aggregated
	Count int
}

// AggregateComputeSavingsPlans aggregates multiple Compute Savings Plans into a single decision.
//
// This is necessary because multiple Compute SPs of the same type would otherwise create
// duplicate overlay names. We simply sum up the remaining capacities.
//
// Phase 2 logic: If there's ANY remaining capacity > 0, create the overlay.
func AggregateComputeSavingsPlans(
	utilizations []prometheus.SavingsPlanUtilization,
	capacities []prometheus.SavingsPlanCapacity,
) AggregatedSavingsPlan {
	if len(capacities) == 0 {
		return AggregatedSavingsPlan{}
	}

	agg := AggregatedSavingsPlan{
		Type:      prometheus.SavingsPlanTypeCompute,
		AccountID: capacities[0].AccountID,
	}

	// Sum up both remaining capacity and hourly commitment
	for _, cap := range capacities {
		agg.TotalRemainingCapacity += cap.RemainingCapacity
		agg.TotalHourlyCommitment += cap.HourlyCommitment
		agg.Count++
	}

	// Calculate utilization: (1 - remaining/commitment) * 100
	if agg.TotalHourlyCommitment > 0 {
		agg.UtilizationPercent = (1 - (agg.TotalRemainingCapacity / agg.TotalHourlyCommitment)) * 100
	}

	return agg
}

// AggregateEC2InstanceSavingsPlans aggregates EC2 Instance Savings Plans by instance family and region.
//
// Returns a map of "family:region" -> aggregated metrics. Multiple SPs for the same family+region
// are summed together to prevent duplicate overlay names.
//
// EC2 Instance SPs are scoped to ONE family + ONE region, so we must group by both dimensions.
// Example keys: "m5:us-west-2", "c5:us-east-1"
//
// Phase 2 logic: Just sum up remaining capacity per family+region.
func AggregateEC2InstanceSavingsPlans(
	utilizations []prometheus.SavingsPlanUtilization,
	capacities []prometheus.SavingsPlanCapacity,
) map[string]AggregatedSavingsPlan {
	// Group capacities by instance family AND region (composite key)
	// EC2 Instance SPs are scoped to one family + one region, so we must group by both
	result := make(map[string]AggregatedSavingsPlan)

	for _, cap := range capacities {
		// Create composite key: "family:region"
		key := cap.InstanceFamily + ":" + cap.Region

		agg, exists := result[key]
		if !exists {
			agg = AggregatedSavingsPlan{
				Type:           prometheus.SavingsPlanTypeEC2Instance,
				InstanceFamily: cap.InstanceFamily,
				Region:         cap.Region,
				AccountID:      cap.AccountID,
			}
		}

		// Sum up both remaining capacity and hourly commitment
		agg.TotalRemainingCapacity += cap.RemainingCapacity
		agg.TotalHourlyCommitment += cap.HourlyCommitment
		agg.Count++

		result[key] = agg
	}

	// Calculate utilization for each aggregated family: (1 - remaining/commitment) * 100
	for key, agg := range result {
		if agg.TotalHourlyCommitment > 0 {
			agg.UtilizationPercent = (1 - (agg.TotalRemainingCapacity / agg.TotalHourlyCommitment)) * 100
			result[key] = agg
		}
	}

	return result
}

// AggregatedReservedInstance represents aggregated RI metrics for a single instance type.
type AggregatedReservedInstance struct {
	// AccountID is the AWS account ID
	AccountID string

	// Region is the AWS region
	Region string

	// InstanceType is the EC2 instance type
	InstanceType string

	// TotalCount is the sum of all RI counts across all AZs
	TotalCount int
}

// AggregateReservedInstances aggregates Reserved Instances by instance type and region.
//
// RIs can exist in multiple AZs within the same region, so we sum the counts per instance type+region
// to prevent duplicate overlay names (one overlay per instance type+region, not per AZ).
//
// RIs are scoped to ONE instance type + ONE region + ONE account, so we must group by type and region.
// Example keys: "m5.xlarge:us-west-2", "c5.2xlarge:us-east-1"
func AggregateReservedInstances(ris []prometheus.ReservedInstance) map[string]AggregatedReservedInstance {
	byTypeRegion := make(map[string]AggregatedReservedInstance)

	for _, ri := range ris {
		// Create composite key: "instanceType:region"
		key := ri.InstanceType + ":" + ri.Region

		agg, exists := byTypeRegion[key]
		if !exists {
			agg = AggregatedReservedInstance{
				AccountID:    ri.AccountID,
				Region:       ri.Region,
				InstanceType: ri.InstanceType,
			}
		}

		agg.TotalCount += ri.Count
		byTypeRegion[key] = agg
	}

	return byTypeRegion
}

// AnalyzeComputeSavingsPlan determines if a global Compute SP overlay should exist.
//
// Compute SPs apply to ALL instance families and ALL regions, so the overlay targets
// all on-demand instances globally (using karpenter.k8s.aws/instance-family: Exists).
//
// This method expects aggregated metrics for proper handling of multiple SPs.
// For single SPs, you can create a simple aggregation or use the convenience wrapper
// AnalyzeComputeSavingsPlanSingle().
func (e *DecisionEngine) AnalyzeComputeSavingsPlan(
	agg AggregatedSavingsPlan,
) Decision {
	// Generate overlay name using configured prefix
	prefix := e.Config.Overlays.Naming.ComputeSavingsPlanPrefix
	if prefix == "" {
		prefix = config.DefaultOverlayNamingComputeSPPrefix
	}
	overlayName := fmt.Sprintf("%s-global", prefix)

	decision := Decision{
		Name:               overlayName,
		CapacityType:       CapacityTypeComputeSavingsPlan,
		Weight:             e.Config.Overlays.Weights.ComputeSavingsPlan,
		Price:              "0.00", // 100% discount for Phase 2
		TargetSelector:     "karpenter.k8s.aws/instance-family: Exists, karpenter.sh/capacity-type: In [on-demand]",
		UtilizationPercent: agg.UtilizationPercent,
		RemainingCapacity:  agg.TotalRemainingCapacity,
	}

	// Decision logic: overlay exists if BOTH conditions are true:
	// 1. Utilization below threshold
	// 2. Remaining capacity available
	threshold := e.Config.Overlays.UtilizationThreshold

	if agg.UtilizationPercent >= threshold {
		decision.ShouldExist = false
		decision.Reason = fmt.Sprintf("utilization %.1f%% at/above threshold %.1f%%", agg.UtilizationPercent, threshold)
	} else if agg.TotalRemainingCapacity <= 0 {
		decision.ShouldExist = false
		decision.Reason = fmt.Sprintf("no remaining capacity (%.2f $/hour)", agg.TotalRemainingCapacity)
	} else {
		decision.ShouldExist = true
		decision.Reason = fmt.Sprintf("utilization %.1f%% below threshold %.1f%%, capacity available (%.2f $/hour)",
			agg.UtilizationPercent, threshold, agg.TotalRemainingCapacity)
	}

	return decision
}

// AnalyzeEC2InstanceSavingsPlan determines if a family-specific EC2 Instance SP overlay should exist.
//
// EC2 Instance SPs apply to a specific instance family in a specific region.
//
// NOTE: This method now expects aggregated metrics. Call AggregateEC2InstanceSavingsPlans()
// first to combine multiple EC2 Instance SPs for the same family+region before calling this method.
func (e *DecisionEngine) AnalyzeEC2InstanceSavingsPlan(
	agg AggregatedSavingsPlan,
) Decision {
	// Generate unique name per family and region using configured prefix
	prefix := e.Config.Overlays.Naming.EC2InstanceSavingsPlanPrefix
	if prefix == "" {
		prefix = config.DefaultOverlayNamingEC2InstanceSPPrefix
	}
	overlayName := fmt.Sprintf("%s-%s-%s", prefix, agg.InstanceFamily, agg.Region)

	decision := Decision{
		Name:         overlayName,
		CapacityType: CapacityTypeEC2InstanceSavingsPlan,
		Weight:       e.Config.Overlays.Weights.EC2InstanceSavingsPlan,
		Price:        "0.00", // 100% discount for Phase 2
		TargetSelector: fmt.Sprintf(
			"karpenter.k8s.aws/instance-family: In [%s], karpenter.sh/capacity-type: In [on-demand]",
			agg.InstanceFamily,
		),
		UtilizationPercent: agg.UtilizationPercent,
		RemainingCapacity:  agg.TotalRemainingCapacity,
	}

	threshold := e.Config.Overlays.UtilizationThreshold

	if agg.UtilizationPercent >= threshold {
		decision.ShouldExist = false
		decision.Reason = fmt.Sprintf("utilization %.1f%% at/above threshold %.1f%%", agg.UtilizationPercent, threshold)
	} else if agg.TotalRemainingCapacity <= 0 {
		decision.ShouldExist = false
		decision.Reason = fmt.Sprintf("no remaining capacity (%.2f $/hour)", agg.TotalRemainingCapacity)
	} else {
		decision.ShouldExist = true
		decision.Reason = fmt.Sprintf("utilization %.1f%% below threshold %.1f%%, capacity available (%.2f $/hour)",
			agg.UtilizationPercent, threshold, agg.TotalRemainingCapacity)
	}

	return decision
}

// AnalyzeReservedInstance determines if an instance-type-specific RI overlay should exist.
//
// RIs are binary: either available (count > 0) or not. No utilization percentage.
//
// NOTE: This method now expects aggregated metrics. Call AggregateReservedInstances()
// first to combine multiple RIs for the same instance type+region across AZs before calling this method.
func (e *DecisionEngine) AnalyzeReservedInstance(agg AggregatedReservedInstance) Decision {
	// Generate unique name per instance type and region using configured prefix
	prefix := e.Config.Overlays.Naming.ReservedInstancePrefix
	if prefix == "" {
		prefix = config.DefaultOverlayNamingReservedInstancePrefix
	}
	overlayName := fmt.Sprintf("%s-%s-%s", prefix, agg.InstanceType, agg.Region)

	decision := Decision{
		Name:         overlayName,
		CapacityType: CapacityTypeReservedInstance,
		Weight:       e.Config.Overlays.Weights.ReservedInstance,
		Price:        "0.00", // 100% discount for Phase 2
		TargetSelector: fmt.Sprintf("node.kubernetes.io/instance-type: In [%s], karpenter.sh/capacity-type: In [on-demand]",
			agg.InstanceType),
		UtilizationPercent: 0, // RIs don't have utilization metrics
		RemainingCapacity:  0, // RIs tracked by count, not $/hour
	}

	// Decision logic: overlay exists if RI count > 0
	if agg.TotalCount > 0 {
		decision.ShouldExist = true
		decision.Reason = fmt.Sprintf("%d reserved instances available", agg.TotalCount)
	} else {
		decision.ShouldExist = false
		decision.Reason = "no reserved instances available"
	}

	return decision
}

// AnalyzeComputeSavingsPlanSingle is a convenience wrapper for analyzing a single Compute SP.
// For production code with multiple SPs, use AggregateComputeSavingsPlans() + AnalyzeComputeSavingsPlan().
func (e *DecisionEngine) AnalyzeComputeSavingsPlanSingle(
	utilization prometheus.SavingsPlanUtilization,
	capacity prometheus.SavingsPlanCapacity,
) Decision {
	// Create single-item aggregation
	agg := AggregateComputeSavingsPlans(
		[]prometheus.SavingsPlanUtilization{utilization},
		[]prometheus.SavingsPlanCapacity{capacity},
	)
	return e.AnalyzeComputeSavingsPlan(agg)
}

// AnalyzeEC2InstanceSavingsPlanSingle is a convenience wrapper for analyzing a single EC2 Instance SP.
// For production code with multiple SPs, use AggregateEC2InstanceSavingsPlans() + AnalyzeEC2InstanceSavingsPlan().
func (e *DecisionEngine) AnalyzeEC2InstanceSavingsPlanSingle(
	utilization prometheus.SavingsPlanUtilization,
	capacity prometheus.SavingsPlanCapacity,
) Decision {
	// Create single-item aggregation
	aggByFamily := AggregateEC2InstanceSavingsPlans(
		[]prometheus.SavingsPlanUtilization{utilization},
		[]prometheus.SavingsPlanCapacity{capacity},
	)
	// Return the single aggregated result
	for _, agg := range aggByFamily {
		return e.AnalyzeEC2InstanceSavingsPlan(agg)
	}
	// Should never reach here if inputs are valid
	return Decision{}
}

// AnalyzeReservedInstanceSingle is a convenience wrapper for analyzing a single RI.
// For production code with multiple RIs, use AggregateReservedInstances() + AnalyzeReservedInstance().
func (e *DecisionEngine) AnalyzeReservedInstanceSingle(ri prometheus.ReservedInstance) Decision {
	// Create single-item aggregation
	aggByType := AggregateReservedInstances([]prometheus.ReservedInstance{ri})
	// Return the single aggregated result
	for _, agg := range aggByType {
		return e.AnalyzeReservedInstance(agg)
	}
	// Should never reach here if inputs are valid
	return Decision{}
}
