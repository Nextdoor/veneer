/*
Copyright 2025 Karve Contributors.

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

	"github.com/nextdoor/karve/pkg/config"
	"github.com/nextdoor/karve/pkg/prometheus"
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

// AnalyzeComputeSavingsPlan determines if a global Compute SP overlay should exist.
//
// Compute SPs apply to ALL instance families and ALL regions, so the overlay targets
// all on-demand instances globally (using karpenter.k8s.aws/instance-family: Exists).
func (e *DecisionEngine) AnalyzeComputeSavingsPlan(
	utilization prometheus.SavingsPlanUtilization,
	capacity prometheus.SavingsPlanCapacity,
) Decision {
	decision := Decision{
		Name:               "cost-aware-compute-sp-global",
		CapacityType:       CapacityTypeComputeSavingsPlan,
		Weight:             e.Config.OverlayManagement.Weights.ComputeSavingsPlan,
		Price:              "0.00", // 100% discount for Phase 2
		TargetSelector:     "karpenter.k8s.aws/instance-family: Exists, karpenter.sh/capacity-type: In [on-demand]",
		UtilizationPercent: utilization.UtilizationPercent,
		RemainingCapacity:  capacity.RemainingCapacity,
	}

	// Decision logic: overlay exists if BOTH conditions are true
	// 1. Utilization below threshold
	// 2. Remaining capacity available
	threshold := e.Config.OverlayManagement.UtilizationThreshold

	if utilization.UtilizationPercent >= threshold {
		decision.ShouldExist = false
		decision.Reason = fmt.Sprintf("utilization %.1f%% at/above threshold %.1f%%", utilization.UtilizationPercent, threshold)
	} else if capacity.RemainingCapacity <= 0 {
		decision.ShouldExist = false
		decision.Reason = fmt.Sprintf("no remaining capacity (%.2f $/hour)", capacity.RemainingCapacity)
	} else {
		decision.ShouldExist = true
		decision.Reason = fmt.Sprintf("utilization %.1f%% below threshold %.1f%%, capacity available (%.2f $/hour)",
			utilization.UtilizationPercent, threshold, capacity.RemainingCapacity)
	}

	return decision
}

// AnalyzeEC2InstanceSavingsPlan determines if a family-specific EC2 Instance SP overlay should exist.
//
// EC2 Instance SPs apply to a specific instance family in a specific region.
func (e *DecisionEngine) AnalyzeEC2InstanceSavingsPlan(
	utilization prometheus.SavingsPlanUtilization,
	capacity prometheus.SavingsPlanCapacity,
) Decision {
	// Generate unique name per family
	overlayName := fmt.Sprintf("cost-aware-ec2-sp-%s", utilization.InstanceFamily)

	decision := Decision{
		Name:               overlayName,
		CapacityType:       CapacityTypeEC2InstanceSavingsPlan,
		Weight:             e.Config.OverlayManagement.Weights.EC2InstanceSavingsPlan,
		Price:              "0.00", // 100% discount for Phase 2
		TargetSelector:     fmt.Sprintf("karpenter.k8s.aws/instance-family: In [%s], karpenter.sh/capacity-type: In [on-demand]", utilization.InstanceFamily),
		UtilizationPercent: utilization.UtilizationPercent,
		RemainingCapacity:  capacity.RemainingCapacity,
	}

	threshold := e.Config.OverlayManagement.UtilizationThreshold

	if utilization.UtilizationPercent >= threshold {
		decision.ShouldExist = false
		decision.Reason = fmt.Sprintf("utilization %.1f%% at/above threshold %.1f%%", utilization.UtilizationPercent, threshold)
	} else if capacity.RemainingCapacity <= 0 {
		decision.ShouldExist = false
		decision.Reason = fmt.Sprintf("no remaining capacity (%.2f $/hour)", capacity.RemainingCapacity)
	} else {
		decision.ShouldExist = true
		decision.Reason = fmt.Sprintf("utilization %.1f%% below threshold %.1f%%, capacity available (%.2f $/hour)",
			utilization.UtilizationPercent, threshold, capacity.RemainingCapacity)
	}

	return decision
}

// AnalyzeReservedInstance determines if an instance-type-specific RI overlay should exist.
//
// RIs are binary: either available (count > 0) or not. No utilization percentage.
func (e *DecisionEngine) AnalyzeReservedInstance(ri prometheus.ReservedInstance) Decision {
	// Generate unique name per instance type
	overlayName := fmt.Sprintf("cost-aware-ri-%s", ri.InstanceType)

	decision := Decision{
		Name:         overlayName,
		CapacityType: CapacityTypeReservedInstance,
		Weight:       e.Config.OverlayManagement.Weights.ReservedInstance,
		Price:        "0.00", // 100% discount for Phase 2
		TargetSelector: fmt.Sprintf("node.kubernetes.io/instance-type: In [%s], karpenter.sh/capacity-type: In [on-demand]",
			ri.InstanceType),
		UtilizationPercent: 0, // RIs don't have utilization metrics
		RemainingCapacity:  0, // RIs tracked by count, not $/hour
	}

	// Decision logic: overlay exists if RI count > 0
	if ri.Count > 0 {
		decision.ShouldExist = true
		decision.Reason = fmt.Sprintf("%d reserved instances available", ri.Count)
	} else {
		decision.ShouldExist = false
		decision.Reason = "no reserved instances available"
	}

	return decision
}
