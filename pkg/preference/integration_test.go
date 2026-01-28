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
	"testing"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/karpenter/pkg/apis/v1alpha1"
)

// TestIntegration_ParseGenerateValidate tests the full workflow from parsing
// NodePool annotations to generating and validating NodeOverlay resources.
func TestIntegration_ParseGenerateValidate(t *testing.T) {
	tests := []struct {
		name         string
		annotations  map[string]string
		nodePoolName string
		disabled     bool
		wantOverlays int
		check        func(t *testing.T, overlays []*v1alpha1.NodeOverlay)
	}{
		{
			name: "single preference workflow",
			annotations: map[string]string{
				"veneer.io/preference.1": "karpenter.k8s.aws/instance-family=c7a,c7g adjust=-20%",
			},
			nodePoolName: "test-workload",
			disabled:     false,
			wantOverlays: 1,
			check: func(t *testing.T, overlays []*v1alpha1.NodeOverlay) {
				o := overlays[0]

				// Verify name
				if o.Name != "pref-test-workload-1" {
					t.Errorf("expected name pref-test-workload-1, got %s", o.Name)
				}

				// Verify labels
				if o.Labels[LabelManagedBy] != LabelManagedByValue {
					t.Errorf("missing or incorrect managed-by label")
				}
				if o.Labels[LabelPreferenceType] != LabelPreferenceTypeValue {
					t.Errorf("missing or incorrect type label")
				}
				if o.Labels[LabelSourceNodePool] != "test-workload" {
					t.Errorf("missing or incorrect source-nodepool label")
				}

				// Verify priceAdjustment
				if o.Spec.PriceAdjustment == nil || *o.Spec.PriceAdjustment != "-20%" {
					t.Errorf("expected priceAdjustment -20%%, got %v", o.Spec.PriceAdjustment)
				}

				// Verify weight matches preference number
				if o.Spec.Weight == nil || *o.Spec.Weight != 1 {
					t.Errorf("expected weight 1, got %v", o.Spec.Weight)
				}

				// Verify requirements include nodepool scope
				var hasNodePoolReq bool
				var hasInstanceFamilyReq bool
				for _, req := range o.Spec.Requirements {
					if req.Key == LabelNodePool {
						hasNodePoolReq = true
						if req.Operator != corev1.NodeSelectorOpIn {
							t.Errorf("nodepool requirement should be In operator")
						}
						if len(req.Values) != 1 || req.Values[0] != "test-workload" {
							t.Errorf("nodepool requirement should have value test-workload")
						}
					}
					if req.Key == LabelInstanceFamily {
						hasInstanceFamilyReq = true
						if len(req.Values) != 2 {
							t.Errorf("expected 2 instance families, got %d", len(req.Values))
						}
					}
				}
				if !hasNodePoolReq {
					t.Error("missing nodepool requirement")
				}
				if !hasInstanceFamilyReq {
					t.Error("missing instance-family requirement")
				}
			},
		},
		{
			name: "multiple preferences with different weights",
			annotations: map[string]string{
				"veneer.io/preference.1": "karpenter.k8s.aws/instance-family=c7a adjust=-10%",
				"veneer.io/preference.5": "kubernetes.io/arch=arm64 adjust=-30%",
				"veneer.io/preference.3": "karpenter.sh/capacity-type=spot adjust=+20%",
			},
			nodePoolName: "multi-pref",
			disabled:     false,
			wantOverlays: 3,
			check: func(t *testing.T, overlays []*v1alpha1.NodeOverlay) {
				// Overlays should be generated in preference number order
				expectedWeights := []int32{1, 3, 5}
				expectedAdj := []string{"-10%", "+20%", "-30%"}

				for i, o := range overlays {
					if o.Spec.Weight == nil || *o.Spec.Weight != expectedWeights[i] {
						t.Errorf("overlay %d: expected weight %d, got %v", i, expectedWeights[i], o.Spec.Weight)
					}
					if o.Spec.PriceAdjustment == nil || *o.Spec.PriceAdjustment != expectedAdj[i] {
						t.Errorf("overlay %d: expected adjustment %s, got %v", i, expectedAdj[i], o.Spec.PriceAdjustment)
					}
				}
			},
		},
		{
			name: "multi-matcher preference",
			annotations: map[string]string{
				"veneer.io/preference.1": "karpenter.k8s.aws/instance-family=m7g kubernetes.io/arch=arm64 adjust=-25%",
			},
			nodePoolName: "multi-match",
			disabled:     false,
			wantOverlays: 1,
			check: func(t *testing.T, overlays []*v1alpha1.NodeOverlay) {
				o := overlays[0]
				// Should have 3 requirements: nodepool + 2 matchers
				if len(o.Spec.Requirements) != 3 {
					t.Errorf("expected 3 requirements, got %d", len(o.Spec.Requirements))
				}

				var hasFamily, hasArch bool
				for _, req := range o.Spec.Requirements {
					if req.Key == LabelInstanceFamily {
						hasFamily = true
					}
					if req.Key == LabelArch {
						hasArch = true
					}
				}
				if !hasFamily {
					t.Error("missing instance-family requirement")
				}
				if !hasArch {
					t.Error("missing arch requirement")
				}
			},
		},
		{
			name: "disabled mode adds impossible requirement",
			annotations: map[string]string{
				"veneer.io/preference.1": "karpenter.k8s.aws/instance-family=c7a adjust=-20%",
			},
			nodePoolName: "disabled-test",
			disabled:     true,
			wantOverlays: 1,
			check: func(t *testing.T, overlays []*v1alpha1.NodeOverlay) {
				o := overlays[0]

				// Check disabled label
				if o.Labels[LabelDisabledKey] != LabelDisabledValue {
					t.Errorf("expected disabled label, got %v", o.Labels[LabelDisabledKey])
				}

				// Check for impossible requirement
				var hasDisabledReq bool
				for _, req := range o.Spec.Requirements {
					if req.Key == LabelDisabledKey {
						hasDisabledReq = true
						if len(req.Values) != 1 || req.Values[0] != LabelDisabledValue {
							t.Errorf("disabled requirement should have value 'true'")
						}
					}
				}
				if !hasDisabledReq {
					t.Error("missing disabled requirement")
				}
			},
		},
		{
			name: "operators are correctly mapped",
			annotations: map[string]string{
				"veneer.io/preference.1": "karpenter.k8s.aws/instance-family!=m5,m6i adjust=+10%",
				"veneer.io/preference.2": "karpenter.k8s.aws/instance-cpu>4 adjust=-15%",
				"veneer.io/preference.3": "karpenter.k8s.aws/instance-memory<32768 adjust=-5%",
			},
			nodePoolName: "operators",
			disabled:     false,
			wantOverlays: 3,
			check: func(t *testing.T, overlays []*v1alpha1.NodeOverlay) {
				// First overlay: NotIn operator
				for _, req := range overlays[0].Spec.Requirements {
					if req.Key == LabelInstanceFamily {
						if req.Operator != corev1.NodeSelectorOpNotIn {
							t.Errorf("preference 1: expected NotIn operator, got %s", req.Operator)
						}
					}
				}

				// Second overlay: Gt operator
				for _, req := range overlays[1].Spec.Requirements {
					if req.Key == LabelInstanceCPU {
						if req.Operator != corev1.NodeSelectorOpGt {
							t.Errorf("preference 2: expected Gt operator, got %s", req.Operator)
						}
					}
				}

				// Third overlay: Lt operator
				for _, req := range overlays[2].Spec.Requirements {
					if req.Key == LabelInstanceMemory {
						if req.Operator != corev1.NodeSelectorOpLt {
							t.Errorf("preference 3: expected Lt operator, got %s", req.Operator)
						}
					}
				}
			},
		},
		{
			name: "no preferences returns empty",
			annotations: map[string]string{
				"other-annotation": "value",
			},
			nodePoolName: "no-prefs",
			disabled:     false,
			wantOverlays: 0,
		},
		{
			name: "partial success - only valid preferences generate overlays",
			annotations: map[string]string{
				"veneer.io/preference.1": "karpenter.k8s.aws/instance-family=c7a adjust=-20%",
				"veneer.io/preference.2": "invalid-no-adjust",
				"veneer.io/preference.3": "unsupported.label/foo=bar adjust=-10%",
			},
			nodePoolName: "partial",
			disabled:     false,
			wantOverlays: 1, // Only preference.1 is valid
			check: func(t *testing.T, overlays []*v1alpha1.NodeOverlay) {
				if overlays[0].Name != "pref-partial-1" {
					t.Errorf("expected overlay from preference 1, got %s", overlays[0].Name)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Step 1: Parse preferences from annotations
			prefs, parseErrors := ParseNodePoolPreferences(tt.annotations, tt.nodePoolName)

			// Log parse errors for debugging
			for _, err := range parseErrors {
				t.Logf("Parse error (may be expected): %v", err)
			}

			// Step 2: Generate overlays from preferences
			generator := NewGeneratorWithOptions(tt.disabled)
			overlays := generator.GenerateAll(prefs)

			// Step 3: Verify number of overlays
			if len(overlays) != tt.wantOverlays {
				t.Errorf("expected %d overlays, got %d", tt.wantOverlays, len(overlays))
			}

			// Step 4: Run custom checks if provided
			if tt.check != nil && len(overlays) > 0 {
				tt.check(t, overlays)
			}

			// Step 5: Verify all overlays have required fields
			for i, o := range overlays {
				if o.Name == "" {
					t.Errorf("overlay %d: missing name", i)
				}
				if o.Labels == nil || len(o.Labels) == 0 {
					t.Errorf("overlay %d: missing labels", i)
				}
				if o.Spec.Requirements == nil || len(o.Spec.Requirements) == 0 {
					t.Errorf("overlay %d: missing requirements", i)
				}
				if o.Spec.PriceAdjustment == nil {
					t.Errorf("overlay %d: missing priceAdjustment", i)
				}
				if o.Spec.Weight == nil {
					t.Errorf("overlay %d: missing weight", i)
				}
			}
		})
	}
}

// TestIntegration_HelperFunctions tests the helper functions for working with
// preference overlays.
func TestIntegration_HelperFunctions(t *testing.T) {
	// Parse a preference
	annotations := map[string]string{
		"veneer.io/preference.5": "karpenter.k8s.aws/instance-family=c7a adjust=-20%",
	}
	prefs, _ := ParseNodePoolPreferences(annotations, "helper-test")

	// Generate overlay
	generator := NewGenerator()
	overlays := generator.GenerateAll(prefs)

	if len(overlays) != 1 {
		t.Fatalf("expected 1 overlay, got %d", len(overlays))
	}

	overlay := overlays[0]

	// Test IsPreferenceOverlay
	if !IsPreferenceOverlay(overlay) {
		t.Error("IsPreferenceOverlay should return true for generated overlay")
	}

	// Test GetSourceNodePool
	sourcePool := GetSourceNodePool(overlay)
	if sourcePool != "helper-test" {
		t.Errorf("expected source pool 'helper-test', got %s", sourcePool)
	}

	// Test GetPreferenceNumber
	prefNum := GetPreferenceNumber(overlay)
	if prefNum != 5 {
		t.Errorf("expected preference number 5, got %d", prefNum)
	}

	// Test OverlayNameForPreference
	expectedName := OverlayNameForPreference("helper-test", 5)
	if overlay.Name != expectedName {
		t.Errorf("expected name %s, got %s", expectedName, overlay.Name)
	}
}
