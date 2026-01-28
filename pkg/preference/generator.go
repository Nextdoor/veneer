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

package preference

import (
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	karpenterv1alpha1 "sigs.k8s.io/karpenter/pkg/apis/v1alpha1"
)

// Generator creates Karpenter NodeOverlay resources from parsed preferences.
//
// The generator converts Preference structs into fully-formed NodeOverlay Kubernetes
// resources with priceAdjustment to influence Karpenter's instance selection.
type Generator struct {
	// Disabled controls whether generated overlays are active or inactive.
	// When true, an impossible requirement is added to each overlay that prevents
	// it from matching any nodes. This allows testing overlay creation without
	// affecting Karpenter's provisioning decisions.
	Disabled bool
}

// NewGenerator creates a new preference overlay generator with default settings (enabled).
func NewGenerator() *Generator {
	return &Generator{Disabled: false}
}

// NewGeneratorWithOptions creates a new preference overlay generator with the specified options.
func NewGeneratorWithOptions(disabled bool) *Generator {
	return &Generator{Disabled: disabled}
}

// Generate creates a NodeOverlay from a Preference.
//
// The generated overlay includes:
//   - Name following the pattern: pref-{nodepool}-{number}
//   - Labels for identification and debugging
//   - Requirements scoped to the source NodePool plus user-specified matchers
//   - PriceAdjustment from the preference (e.g., "-20%")
//   - Weight equal to the preference number
func (g *Generator) Generate(pref Preference) *karpenterv1alpha1.NodeOverlay {
	overlay := &karpenterv1alpha1.NodeOverlay{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "karpenter.sh/v1alpha1",
			Kind:       "NodeOverlay",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   g.generateName(pref),
			Labels: g.generateLabels(pref),
		},
		Spec: karpenterv1alpha1.NodeOverlaySpec{
			Requirements:    g.generateRequirements(pref),
			PriceAdjustment: g.formatPriceAdjustment(pref.Adjustment),
			Weight:          int32Ptr(int32(pref.Number)),
		},
	}

	return overlay
}

// GenerateAll creates NodeOverlays for all preferences.
//
// Returns a slice of NodeOverlay resources, one per preference.
func (g *Generator) GenerateAll(prefs []Preference) []*karpenterv1alpha1.NodeOverlay {
	overlays := make([]*karpenterv1alpha1.NodeOverlay, 0, len(prefs))
	for _, pref := range prefs {
		overlays = append(overlays, g.Generate(pref))
	}
	return overlays
}

// generateName creates the overlay name from a preference.
// Format: pref-{nodepool}-{number}
func (g *Generator) generateName(pref Preference) string {
	return fmt.Sprintf("pref-%s-%d", pref.NodePoolName, pref.Number)
}

// OverlayNameForPreference returns the expected overlay name for a preference.
// This is useful for reconciliation to find existing overlays.
func OverlayNameForPreference(nodePoolName string, preferenceNumber int) string {
	return fmt.Sprintf("pref-%s-%d", nodePoolName, preferenceNumber)
}

// generateLabels creates the label map for a preference overlay.
//
// Labels include:
//   - managed-by: veneer
//   - veneer.io/type: preference
//   - veneer.io/source-nodepool: {nodepool name}
//   - veneer.io/preference-number: {number}
//   - veneer.io/disabled: "true" (only if disabled mode is enabled)
func (g *Generator) generateLabels(pref Preference) map[string]string {
	labels := map[string]string{
		LabelManagedBy:        LabelManagedByValue,
		LabelPreferenceType:   LabelPreferenceTypeValue,
		LabelSourceNodePool:   pref.NodePoolName,
		LabelPreferenceNumber: strconv.Itoa(pref.Number),
	}

	if g.Disabled {
		labels[LabelDisabledKey] = LabelDisabledValue
	}

	return labels
}

// generateRequirements creates the NodeSelectorRequirements for a preference overlay.
//
// Requirements always include:
//   - karpenter.sh/nodepool In [nodepool name] - scope to source NodePool
//   - user-specified matchers converted to requirements
//   - veneer.io/disabled: "true" (if disabled mode is enabled)
func (g *Generator) generateRequirements(pref Preference) []corev1.NodeSelectorRequirement {
	// Pre-allocate: 1 for nodepool + matchers + potentially 1 for disabled
	capacity := 1 + len(pref.Matchers)
	if g.Disabled {
		capacity++
	}
	requirements := make([]corev1.NodeSelectorRequirement, 0, capacity)

	// If disabled mode is enabled, add an impossible requirement first.
	// This prevents the overlay from ever matching any instances.
	if g.Disabled {
		requirements = append(requirements, corev1.NodeSelectorRequirement{
			Key:      LabelDisabledKey,
			Operator: corev1.NodeSelectorOpIn,
			Values:   []string{LabelDisabledValue},
		})
	}

	// Always scope to the source NodePool
	requirements = append(requirements, corev1.NodeSelectorRequirement{
		Key:      LabelNodePool,
		Operator: corev1.NodeSelectorOpIn,
		Values:   []string{pref.NodePoolName},
	})

	// Convert user matchers to requirements
	for _, matcher := range pref.Matchers {
		req := corev1.NodeSelectorRequirement{
			Key:    matcher.Key,
			Values: matcher.Values,
		}

		// Map our operators to Kubernetes operators
		switch matcher.Operator {
		case OperatorIn:
			req.Operator = corev1.NodeSelectorOpIn
		case OperatorNotIn:
			req.Operator = corev1.NodeSelectorOpNotIn
		case OperatorGt:
			req.Operator = corev1.NodeSelectorOpGt
		case OperatorLt:
			req.Operator = corev1.NodeSelectorOpLt
		default:
			req.Operator = corev1.NodeSelectorOpIn
		}

		requirements = append(requirements, req)
	}

	return requirements
}

// formatPriceAdjustment converts a numeric adjustment to the string format expected
// by Karpenter's priceAdjustment field.
//
// Examples:
//   - -20 -> "-20%"
//   - +40 -> "+40%"
//   - 0 -> "0%"
func (g *Generator) formatPriceAdjustment(adj float64) *string {
	var s string
	if adj > 0 {
		s = fmt.Sprintf("+%.0f%%", adj)
	} else if adj < 0 {
		s = fmt.Sprintf("%.0f%%", adj)
	} else {
		s = "0%"
	}
	return &s
}

// int32Ptr returns a pointer to an int32 value.
func int32Ptr(i int32) *int32 {
	return &i
}

// IsPreferenceOverlay returns true if the overlay was created from a preference annotation.
func IsPreferenceOverlay(overlay *karpenterv1alpha1.NodeOverlay) bool {
	if overlay == nil || overlay.Labels == nil {
		return false
	}
	return overlay.Labels[LabelPreferenceType] == LabelPreferenceTypeValue &&
		overlay.Labels[LabelManagedBy] == LabelManagedByValue
}

// GetSourceNodePool returns the NodePool name that a preference overlay was generated from.
// Returns empty string if the overlay is not a preference overlay.
func GetSourceNodePool(overlay *karpenterv1alpha1.NodeOverlay) string {
	if overlay == nil || overlay.Labels == nil {
		return ""
	}
	return overlay.Labels[LabelSourceNodePool]
}

// GetPreferenceNumber returns the preference number from a preference overlay.
// Returns 0 if the overlay is not a preference overlay or the number can't be parsed.
func GetPreferenceNumber(overlay *karpenterv1alpha1.NodeOverlay) int {
	if overlay == nil || overlay.Labels == nil {
		return 0
	}
	numStr := overlay.Labels[LabelPreferenceNumber]
	num, err := strconv.Atoi(numStr)
	if err != nil {
		return 0
	}
	return num
}
