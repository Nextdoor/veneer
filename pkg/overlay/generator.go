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

// Package overlay provides NodeOverlay generation and lifecycle management.
package overlay

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	karpenterv1alpha1 "sigs.k8s.io/karpenter/pkg/apis/v1alpha1"
)

// Label keys used on Veneer-managed NodeOverlays.
// These labels identify overlays created by Veneer and provide metadata for debugging.
const (
	// LabelManagedBy identifies that Veneer manages this NodeOverlay.
	// All Veneer-created overlays will have this label set to "veneer".
	LabelManagedBy = "app.kubernetes.io/managed-by"

	// LabelManagedByValue is the value for the managed-by label.
	LabelManagedByValue = "veneer"

	// LabelInstanceFamily identifies the EC2 instance family this overlay targets.
	// Only set for EC2 Instance SP and RI overlays (not global Compute SP overlays).
	// Example values: "m5", "c5", "r6i"
	LabelInstanceFamily = "veneer.io/instance-family"

	// LabelInstanceType identifies the specific EC2 instance type for RI overlays.
	// Only set for Reserved Instance overlays.
	// Example values: "m5.xlarge", "c5.2xlarge"
	LabelInstanceType = "veneer.io/instance-type"

	// LabelCapacityType identifies what type of pre-paid capacity backs this overlay.
	// Values: "compute-savings-plan", "ec2-instance-savings-plan", "reserved-instance"
	LabelCapacityType = "veneer.io/capacity-type"

	// LabelRegion identifies the AWS region this overlay is scoped to.
	// Set for EC2 Instance SP and RI overlays.
	LabelRegion = "veneer.io/region"

	// LabelOptimizationReason explains why this overlay exists.
	// Provides human-readable context for debugging and auditing.
	LabelOptimizationReason = "veneer.io/optimization-reason"
)

// Well-known Kubernetes and Karpenter label keys used in NodeOverlay requirements.
const (
	// LabelInstanceTypeK8s is the standard Kubernetes label for instance type.
	LabelInstanceTypeK8s = "node.kubernetes.io/instance-type"

	// LabelInstanceFamilyKarpenter is the Karpenter label for instance family.
	LabelInstanceFamilyKarpenter = "karpenter.k8s.aws/instance-family"

	// LabelCapacityTypeKarpenter is the Karpenter label for capacity type (spot vs on-demand).
	LabelCapacityTypeKarpenter = "karpenter.sh/capacity-type"

	// LabelDisabledKey is the label key used to create an impossible requirement.
	// When Disabled mode is enabled, overlays include a requirement that this label
	// must equal "true", but no nodes will ever have this label, so the overlay
	// never matches any instances.
	LabelDisabledKey = "veneer.io/disabled"

	// LabelDisabledValue is the value used for the disabled requirement.
	LabelDisabledValue = "true"
)

// Action constants for GeneratedOverlay.
const (
	// ActionCreate indicates a new overlay should be created.
	ActionCreate = "create"

	// ActionDelete indicates an existing overlay should be deleted.
	ActionDelete = "delete"
)

// Generator creates Karpenter NodeOverlay resources from Veneer decisions.
//
// The generator converts Decision structs (which represent whether an overlay should exist)
// into fully-formed NodeOverlay Kubernetes resources.
type Generator struct {
	// Disabled controls whether generated overlays are active or inactive.
	// When true, an impossible requirement is added to each overlay that prevents
	// it from matching any nodes. This allows testing overlay creation without
	// affecting Karpenter's provisioning decisions.
	Disabled bool
}

// NewGenerator creates a new NodeOverlay generator with default settings (enabled).
func NewGenerator() *Generator {
	return &Generator{Disabled: false}
}

// NewGeneratorWithOptions creates a new NodeOverlay generator with the specified options.
func NewGeneratorWithOptions(disabled bool) *Generator {
	return &Generator{Disabled: disabled}
}

// GeneratedOverlay wraps a NodeOverlay with additional metadata for dry-run logging.
type GeneratedOverlay struct {
	// Overlay is the generated Karpenter NodeOverlay resource.
	Overlay *karpenterv1alpha1.NodeOverlay

	// Decision is the source decision that triggered this overlay generation.
	Decision Decision

	// Action describes what should happen to this overlay.
	// Values: "create", "update", "delete", "unchanged"
	Action string
}

// Generate creates a NodeOverlay from a Decision.
//
// This only generates overlays for decisions where ShouldExist is true.
// For decisions where ShouldExist is false, returns nil (the overlay should be deleted).
//
// The generated overlay includes:
//   - Proper naming convention based on capacity type
//   - Labels for identification and debugging
//   - Requirements to target appropriate instances
//   - Price set to "0.00" (pre-paid capacity is effectively free)
//   - Weight based on capacity type priority
func (g *Generator) Generate(decision Decision) *karpenterv1alpha1.NodeOverlay {
	if !decision.ShouldExist {
		return nil
	}

	overlay := &karpenterv1alpha1.NodeOverlay{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "karpenter.sh/v1alpha1",
			Kind:       "NodeOverlay",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   decision.Name,
			Labels: g.generateLabels(decision),
		},
		Spec: karpenterv1alpha1.NodeOverlaySpec{
			Requirements: g.generateRequirements(decision),
			Price:        &decision.Price,
			Weight:       int32Ptr(int32(decision.Weight)),
		},
	}

	return overlay
}

// GenerateAll creates NodeOverlays for all decisions that should exist.
//
// Returns a slice of GeneratedOverlay structs containing the overlay and metadata
// about what action should be taken (create/update/delete).
//
// Decisions where ShouldExist is false are included with Action="delete" to indicate
// the overlay should be removed if it exists in the cluster.
func (g *Generator) GenerateAll(decisions []Decision) []GeneratedOverlay {
	results := make([]GeneratedOverlay, 0, len(decisions))

	for _, decision := range decisions {
		var action string
		var overlay *karpenterv1alpha1.NodeOverlay

		if decision.ShouldExist {
			overlay = g.Generate(decision)
			action = ActionCreate // Will be refined to "update" or "unchanged" when comparing to cluster state
		} else {
			action = ActionDelete
		}

		results = append(results, GeneratedOverlay{
			Overlay:  overlay,
			Decision: decision,
			Action:   action,
		})
	}

	return results
}

// generateLabels creates the label map for a NodeOverlay based on the decision.
//
// All overlays get:
//   - managed-by: veneer
//   - capacity-type: the type of pre-paid capacity
//   - optimization-reason: human-readable explanation
//
// Family-specific overlays (EC2 Instance SP, RI) also get:
//   - instance-family: the EC2 instance family
//   - region: the AWS region
//
// RI overlays additionally get:
//   - instance-type: the specific instance type
//
// When disabled mode is enabled, overlays also get:
//   - veneer.io/disabled: "true"
func (g *Generator) generateLabels(decision Decision) map[string]string {
	labels := map[string]string{
		LabelManagedBy:          LabelManagedByValue,
		LabelCapacityType:       capacityTypeToLabelValue(decision.CapacityType),
		LabelOptimizationReason: sanitizeLabelValue(decision.Reason),
	}

	// Add disabled label when in disabled mode for easy identification
	if g.Disabled {
		labels[LabelDisabledKey] = LabelDisabledValue
	}

	// Extract instance family and type from the decision name for family-specific overlays
	switch decision.CapacityType {
	case CapacityTypeEC2InstanceSavingsPlan:
		family, region := parseEC2InstanceSPName(decision.Name)
		if family != "" {
			labels[LabelInstanceFamily] = family
		}
		if region != "" {
			labels[LabelRegion] = region
		}

	case CapacityTypeReservedInstance:
		instanceType, region := parseRIName(decision.Name)
		if instanceType != "" {
			labels[LabelInstanceType] = instanceType
			// Extract family from instance type (e.g., "m5" from "m5.xlarge")
			if idx := strings.Index(instanceType, "."); idx > 0 {
				labels[LabelInstanceFamily] = instanceType[:idx]
			}
		}
		if region != "" {
			labels[LabelRegion] = region
		}
	}

	return labels
}

// generateRequirements creates the NodeSelectorRequirements for targeting instances.
//
// The requirements determine which instances this overlay applies to:
//   - Compute SP (global): All on-demand instances (instance-family Exists)
//   - EC2 Instance SP: On-demand instances of a specific family
//   - Reserved Instance: On-demand instances of a specific type
//
// All overlays target on-demand capacity type since SPs and RIs only apply to on-demand.
//
// When disabled mode is enabled, an additional "impossible" requirement is added that
// requires the label "veneer.io/disabled: true" to be present on nodes. Since no nodes
// have this label, the overlay will never match any instances, effectively disabling it
// while still allowing the overlay to be created in the cluster for testing/validation.
func (g *Generator) generateRequirements(decision Decision) []karpenterv1alpha1.NodeSelectorRequirement {
	var requirements []karpenterv1alpha1.NodeSelectorRequirement

	// If disabled mode is enabled, add an impossible requirement first.
	// This requirement demands that nodes have a label that no node will ever have,
	// ensuring the overlay never matches any instances while still being valid YAML.
	if g.Disabled {
		requirements = append(requirements, karpenterv1alpha1.NodeSelectorRequirement{
			Key:      LabelDisabledKey,
			Operator: corev1.NodeSelectorOpIn,
			Values:   []string{LabelDisabledValue},
		})
	}

	// All overlays target on-demand instances only
	// SPs and RIs don't apply to spot instances
	capacityTypeReq := karpenterv1alpha1.NodeSelectorRequirement{
		Key:      LabelCapacityTypeKarpenter,
		Operator: corev1.NodeSelectorOpIn,
		Values:   []string{"on-demand"},
	}

	switch decision.CapacityType {
	case CapacityTypeComputeSavingsPlan:
		// Global Compute SPs apply to all instance families
		// Use Exists operator to match any instance family
		requirements = append(requirements,
			karpenterv1alpha1.NodeSelectorRequirement{
				Key:      LabelInstanceFamilyKarpenter,
				Operator: corev1.NodeSelectorOpExists,
			},
			capacityTypeReq,
		)

	case CapacityTypeEC2InstanceSavingsPlan:
		// EC2 Instance SPs are scoped to a specific instance family
		family, _ := parseEC2InstanceSPName(decision.Name)
		requirements = append(requirements,
			karpenterv1alpha1.NodeSelectorRequirement{
				Key:      LabelInstanceFamilyKarpenter,
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{family},
			},
			capacityTypeReq,
		)

	case CapacityTypeReservedInstance:
		// RIs are scoped to a specific instance type
		instanceType, _ := parseRIName(decision.Name)
		requirements = append(requirements,
			karpenterv1alpha1.NodeSelectorRequirement{
				Key:      LabelInstanceTypeK8s,
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{instanceType},
			},
			capacityTypeReq,
		)

	default:
		// Should never happen, but return capacity type requirement as a safeguard
		requirements = append(requirements, capacityTypeReq)
	}

	return requirements
}

// capacityTypeToLabelValue converts a CapacityType to a Kubernetes-safe label value.
func capacityTypeToLabelValue(ct CapacityType) string {
	switch ct {
	case CapacityTypeComputeSavingsPlan:
		return "compute-savings-plan"
	case CapacityTypeEC2InstanceSavingsPlan:
		return "ec2-instance-savings-plan"
	case CapacityTypeReservedInstance:
		return "reserved-instance"
	default:
		return "unknown"
	}
}

// sanitizeLabelValue ensures a string is valid as a Kubernetes label value.
// Label values must be 63 characters or less and match the regex:
// [a-z0-9A-Z]([a-z0-9A-Z-_.]*[a-z0-9A-Z])?
func sanitizeLabelValue(s string) string {
	// Replace spaces and special characters with hyphens
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '-'
	}, s)

	// Remove consecutive hyphens
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}

	// Trim leading/trailing hyphens
	s = strings.Trim(s, "-_.")

	// Truncate to 63 characters
	if len(s) > 63 {
		s = s[:63]
		// Ensure we don't end with a hyphen after truncation
		s = strings.TrimRight(s, "-_.")
	}

	// If empty after sanitization, return a default
	if s == "" {
		return "capacity-available"
	}

	return s
}

// parseEC2InstanceSPName extracts the instance family and region from an EC2 Instance SP overlay name.
// Expected format: "cost-aware-ec2-sp-{family}-{region}" (e.g., "cost-aware-ec2-sp-m5-us-west-2")
func parseEC2InstanceSPName(name string) (family, region string) {
	// Remove the prefix to get "{family}-{region}"
	const prefix = "cost-aware-ec2-sp-"
	if !strings.HasPrefix(name, prefix) {
		return "", ""
	}

	remainder := strings.TrimPrefix(name, prefix)

	// The family is everything before the last hyphen-delimited region
	// Regions are like "us-west-2", "eu-central-1", etc.
	// So we need to find where the family ends and the region begins
	// Family examples: "m5", "c5", "r6i", "m5a"
	// This is tricky because both can contain hyphens

	// Strategy: assume the region is the last 3 hyphen-separated segments
	// (e.g., "us-west-2" = "us" + "west" + "2")
	parts := strings.Split(remainder, "-")
	if len(parts) >= 4 {
		// Last 3 parts are the region
		family = strings.Join(parts[:len(parts)-3], "-")
		region = strings.Join(parts[len(parts)-3:], "-")
	} else if len(parts) >= 2 {
		// Fallback: first part is family, rest is region
		family = parts[0]
		region = strings.Join(parts[1:], "-")
	}

	return family, region
}

// parseRIName extracts the instance type and region from a Reserved Instance overlay name.
// Expected format: "cost-aware-ri-{instance-type}-{region}" (e.g., "cost-aware-ri-m5.xlarge-us-west-2")
func parseRIName(name string) (instanceType, region string) {
	// Remove the prefix to get "{instance-type}-{region}"
	const prefix = "cost-aware-ri-"
	if !strings.HasPrefix(name, prefix) {
		return "", ""
	}

	remainder := strings.TrimPrefix(name, prefix)

	// Instance types contain a dot (e.g., "m5.xlarge", "c5.2xlarge")
	// Find the dot to identify where the instance type ends
	dotIdx := strings.Index(remainder, ".")
	if dotIdx < 0 {
		return "", ""
	}

	// Find the next hyphen after the dot to separate instance type from region
	// The instance type is "m5.xlarge", and everything after the next hyphen is region
	afterDot := remainder[dotIdx+1:]
	hyphenIdx := strings.Index(afterDot, "-")
	if hyphenIdx < 0 {
		// No region specified
		return remainder, ""
	}

	instanceType = remainder[:dotIdx+1+hyphenIdx]
	region = afterDot[hyphenIdx+1:]

	return instanceType, region
}

// int32Ptr returns a pointer to an int32 value.
func int32Ptr(i int32) *int32 {
	return &i
}

// ValidationError represents an error found during overlay validation.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidateOverlay checks that a generated NodeOverlay is valid and will be accepted by Kubernetes.
//
// This performs client-side validation before attempting to create the resource,
// catching errors early in dry-run mode.
//
// Validates:
//   - Name is valid (DNS subdomain name)
//   - Labels are valid (keys and values)
//   - Requirements are properly formed
//   - Price is in valid format
//   - Weight is within bounds (1-10000)
func ValidateOverlay(overlay *karpenterv1alpha1.NodeOverlay) []ValidationError {
	var errors []ValidationError

	if overlay == nil {
		return []ValidationError{{Field: "overlay", Message: "overlay is nil"}}
	}

	// Validate name (must be a valid DNS subdomain name)
	if overlay.Name == "" {
		errors = append(errors, ValidationError{Field: "metadata.name", Message: "name is required"})
	} else if len(overlay.Name) > 253 {
		errors = append(errors, ValidationError{Field: "metadata.name", Message: "name must be 253 characters or less"})
	}

	// Validate labels
	for k, v := range overlay.Labels {
		if len(k) > 253 {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("metadata.labels[%s]", k),
				Message: "label key must be 253 characters or less",
			})
		}
		if len(v) > 63 {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("metadata.labels[%s]", k),
				Message: "label value must be 63 characters or less",
			})
		}
	}

	// Validate requirements
	if len(overlay.Spec.Requirements) == 0 {
		errors = append(errors, ValidationError{
			Field:   "spec.requirements",
			Message: "at least one requirement is required",
		})
	}
	for i, req := range overlay.Spec.Requirements {
		if req.Key == "" {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("spec.requirements[%d].key", i),
				Message: "requirement key is required",
			})
		}
		// Validate operator
		validOps := map[corev1.NodeSelectorOperator]bool{
			corev1.NodeSelectorOpIn:           true,
			corev1.NodeSelectorOpNotIn:        true,
			corev1.NodeSelectorOpExists:       true,
			corev1.NodeSelectorOpDoesNotExist: true,
			corev1.NodeSelectorOpGt:           true,
			corev1.NodeSelectorOpLt:           true,
		}
		if !validOps[req.Operator] {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("spec.requirements[%d].operator", i),
				Message: fmt.Sprintf("invalid operator %q", req.Operator),
			})
		}
		// In/NotIn require values
		if (req.Operator == corev1.NodeSelectorOpIn || req.Operator == corev1.NodeSelectorOpNotIn) && len(req.Values) == 0 {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("spec.requirements[%d].values", i),
				Message: fmt.Sprintf("operator %q requires at least one value", req.Operator),
			})
		}
	}

	// Validate price format (must be a non-negative decimal)
	if overlay.Spec.Price != nil {
		price := *overlay.Spec.Price
		// Price must match pattern: ^\d+(\.\d+)?$
		if price == "" {
			errors = append(errors, ValidationError{
				Field:   "spec.price",
				Message: "price cannot be empty string",
			})
		} else {
			valid := true
			dotSeen := false
			for i, ch := range price {
				if ch == '.' {
					if dotSeen || i == 0 || i == len(price)-1 {
						valid = false
						break
					}
					dotSeen = true
				} else if ch < '0' || ch > '9' {
					valid = false
					break
				}
			}
			if !valid {
				errors = append(errors, ValidationError{
					Field:   "spec.price",
					Message: fmt.Sprintf("price %q must be a non-negative decimal (e.g., \"0.00\", \"1.5\")", price),
				})
			}
		}
	}

	// Validate weight (1-10000)
	if overlay.Spec.Weight != nil {
		w := *overlay.Spec.Weight
		if w < 1 || w > 10000 {
			errors = append(errors, ValidationError{
				Field:   "spec.weight",
				Message: fmt.Sprintf("weight %d must be between 1 and 10000", w),
			})
		}
	}

	return errors
}

// FormatOverlayYAML returns a YAML representation of a NodeOverlay for logging.
// This is used in dry-run mode to show what would be created.
func FormatOverlayYAML(overlay *karpenterv1alpha1.NodeOverlay) string {
	if overlay == nil {
		return ""
	}

	var sb strings.Builder

	sb.WriteString("apiVersion: karpenter.sh/v1alpha1\n")
	sb.WriteString("kind: NodeOverlay\n")
	sb.WriteString("metadata:\n")
	sb.WriteString(fmt.Sprintf("  name: %s\n", overlay.Name))

	if len(overlay.Labels) > 0 {
		sb.WriteString("  labels:\n")
		for k, v := range overlay.Labels {
			sb.WriteString(fmt.Sprintf("    %s: %q\n", k, v))
		}
	}

	sb.WriteString("spec:\n")

	if overlay.Spec.Weight != nil {
		sb.WriteString(fmt.Sprintf("  weight: %d\n", *overlay.Spec.Weight))
	}

	if overlay.Spec.Price != nil {
		sb.WriteString(fmt.Sprintf("  price: %q\n", *overlay.Spec.Price))
	}

	if len(overlay.Spec.Requirements) > 0 {
		sb.WriteString("  requirements:\n")
		for _, req := range overlay.Spec.Requirements {
			sb.WriteString(fmt.Sprintf("    - key: %s\n", req.Key))
			sb.WriteString(fmt.Sprintf("      operator: %s\n", req.Operator))
			if len(req.Values) > 0 {
				sb.WriteString("      values:\n")
				for _, v := range req.Values {
					sb.WriteString(fmt.Sprintf("        - %q\n", v))
				}
			}
		}
	}

	return sb.String()
}
