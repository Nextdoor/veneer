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

package overlay

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	karpenterv1alpha1 "sigs.k8s.io/karpenter/pkg/apis/v1alpha1"
)

func TestGenerator_Generate_ComputeSavingsPlan(t *testing.T) {
	g := NewGenerator()

	decision := Decision{
		Name:               "cost-aware-compute-sp-global",
		CapacityType:       CapacityTypeComputeSavingsPlan,
		ShouldExist:        true,
		Weight:             10,
		Price:              "0.00",
		TargetSelector:     "karpenter.k8s.aws/instance-family: Exists, karpenter.sh/capacity-type: In [on-demand]",
		Reason:             "utilization 50.0% below threshold 95.0%, capacity available (25.00 $/hour)",
		UtilizationPercent: 50.0,
		RemainingCapacity:  25.0,
	}

	overlay := g.Generate(decision)

	if overlay == nil {
		t.Fatal("expected overlay to be generated, got nil")
	}

	// Verify metadata
	if overlay.Name != "cost-aware-compute-sp-global" {
		t.Errorf("expected name %q, got %q", "cost-aware-compute-sp-global", overlay.Name)
	}

	// Verify labels
	if overlay.Labels[LabelManagedBy] != LabelManagedByValue {
		t.Errorf("expected managed-by label %q, got %q", LabelManagedByValue, overlay.Labels[LabelManagedBy])
	}
	if overlay.Labels[LabelCapacityType] != "compute-savings-plan" {
		t.Errorf("expected capacity-type label %q, got %q", "compute-savings-plan", overlay.Labels[LabelCapacityType])
	}

	// Verify spec
	if overlay.Spec.Price == nil || *overlay.Spec.Price != "0.00" {
		t.Errorf("expected price %q, got %v", "0.00", overlay.Spec.Price)
	}
	if overlay.Spec.Weight == nil || *overlay.Spec.Weight != 10 {
		t.Errorf("expected weight %d, got %v", 10, overlay.Spec.Weight)
	}

	// Verify requirements - should have instance-family Exists and capacity-type In [on-demand]
	if len(overlay.Spec.Requirements) != 2 {
		t.Fatalf("expected 2 requirements, got %d", len(overlay.Spec.Requirements))
	}

	// Check instance-family requirement
	familyReq := overlay.Spec.Requirements[0]
	if familyReq.Key != LabelInstanceFamilyKarpenter {
		t.Errorf("expected first requirement key %q, got %q", LabelInstanceFamilyKarpenter, familyReq.Key)
	}
	if familyReq.Operator != corev1.NodeSelectorOpExists {
		t.Errorf("expected first requirement operator %q, got %q", corev1.NodeSelectorOpExists, familyReq.Operator)
	}

	// Check capacity-type requirement
	capReq := overlay.Spec.Requirements[1]
	if capReq.Key != LabelCapacityTypeKarpenter {
		t.Errorf("expected second requirement key %q, got %q", LabelCapacityTypeKarpenter, capReq.Key)
	}
	if capReq.Operator != corev1.NodeSelectorOpIn {
		t.Errorf("expected second requirement operator %q, got %q", corev1.NodeSelectorOpIn, capReq.Operator)
	}
	if len(capReq.Values) != 1 || capReq.Values[0] != "on-demand" {
		t.Errorf("expected second requirement values [on-demand], got %v", capReq.Values)
	}
}

func TestGenerator_Generate_EC2InstanceSavingsPlan(t *testing.T) {
	g := NewGenerator()

	decision := Decision{
		Name:               "cost-aware-ec2-sp-m5-us-west-2",
		CapacityType:       CapacityTypeEC2InstanceSavingsPlan,
		ShouldExist:        true,
		Weight:             20,
		Price:              "0.00",
		TargetSelector:     "karpenter.k8s.aws/instance-family: In [m5], karpenter.sh/capacity-type: In [on-demand]",
		Reason:             "utilization 75.0% below threshold 95.0%, capacity available (10.00 $/hour)",
		UtilizationPercent: 75.0,
		RemainingCapacity:  10.0,
	}

	overlay := g.Generate(decision)

	if overlay == nil {
		t.Fatal("expected overlay to be generated, got nil")
	}

	// Verify name
	if overlay.Name != "cost-aware-ec2-sp-m5-us-west-2" {
		t.Errorf("expected name %q, got %q", "cost-aware-ec2-sp-m5-us-west-2", overlay.Name)
	}

	// Verify labels
	if overlay.Labels[LabelInstanceFamily] != "m5" {
		t.Errorf("expected instance-family label %q, got %q", "m5", overlay.Labels[LabelInstanceFamily])
	}
	if overlay.Labels[LabelRegion] != "us-west-2" {
		t.Errorf("expected region label %q, got %q", "us-west-2", overlay.Labels[LabelRegion])
	}
	if overlay.Labels[LabelCapacityType] != "ec2-instance-savings-plan" {
		t.Errorf("expected capacity-type label %q, got %q", "ec2-instance-savings-plan", overlay.Labels[LabelCapacityType])
	}

	// Verify requirements - should target specific family
	if len(overlay.Spec.Requirements) != 2 {
		t.Fatalf("expected 2 requirements, got %d", len(overlay.Spec.Requirements))
	}

	familyReq := overlay.Spec.Requirements[0]
	if familyReq.Key != LabelInstanceFamilyKarpenter {
		t.Errorf("expected first requirement key %q, got %q", LabelInstanceFamilyKarpenter, familyReq.Key)
	}
	if familyReq.Operator != corev1.NodeSelectorOpIn {
		t.Errorf("expected first requirement operator %q, got %q", corev1.NodeSelectorOpIn, familyReq.Operator)
	}
	if len(familyReq.Values) != 1 || familyReq.Values[0] != "m5" {
		t.Errorf("expected first requirement values [m5], got %v", familyReq.Values)
	}
}

func TestGenerator_Generate_ReservedInstance(t *testing.T) {
	g := NewGenerator()

	decision := Decision{
		Name:         "cost-aware-ri-m5.xlarge-us-west-2",
		CapacityType: CapacityTypeReservedInstance,
		ShouldExist:  true,
		Weight:       30,
		Price:        "0.00",
		TargetSelector: "node.kubernetes.io/instance-type: In [m5.xlarge], " +
			"karpenter.sh/capacity-type: In [on-demand]",
		Reason: "3 reserved instances available",
	}

	overlay := g.Generate(decision)

	if overlay == nil {
		t.Fatal("expected overlay to be generated, got nil")
	}

	// Verify name
	if overlay.Name != "cost-aware-ri-m5.xlarge-us-west-2" {
		t.Errorf("expected name %q, got %q", "cost-aware-ri-m5.xlarge-us-west-2", overlay.Name)
	}

	// Verify labels
	if overlay.Labels[LabelInstanceType] != "m5.xlarge" {
		t.Errorf("expected instance-type label %q, got %q", "m5.xlarge", overlay.Labels[LabelInstanceType])
	}
	if overlay.Labels[LabelInstanceFamily] != "m5" {
		t.Errorf("expected instance-family label %q, got %q", "m5", overlay.Labels[LabelInstanceFamily])
	}
	if overlay.Labels[LabelRegion] != "us-west-2" {
		t.Errorf("expected region label %q, got %q", "us-west-2", overlay.Labels[LabelRegion])
	}
	if overlay.Labels[LabelCapacityType] != "reserved-instance" {
		t.Errorf("expected capacity-type label %q, got %q", "reserved-instance", overlay.Labels[LabelCapacityType])
	}

	// Verify requirements - should target specific instance type
	if len(overlay.Spec.Requirements) != 2 {
		t.Fatalf("expected 2 requirements, got %d", len(overlay.Spec.Requirements))
	}

	typeReq := overlay.Spec.Requirements[0]
	if typeReq.Key != LabelInstanceTypeK8s {
		t.Errorf("expected first requirement key %q, got %q", LabelInstanceTypeK8s, typeReq.Key)
	}
	if typeReq.Operator != corev1.NodeSelectorOpIn {
		t.Errorf("expected first requirement operator %q, got %q", corev1.NodeSelectorOpIn, typeReq.Operator)
	}
	if len(typeReq.Values) != 1 || typeReq.Values[0] != "m5.xlarge" {
		t.Errorf("expected first requirement values [m5.xlarge], got %v", typeReq.Values)
	}
}

func TestGenerator_Generate_ShouldNotExist(t *testing.T) {
	g := NewGenerator()

	decision := Decision{
		Name:         "cost-aware-compute-sp-global",
		CapacityType: CapacityTypeComputeSavingsPlan,
		ShouldExist:  false,
		Weight:       10,
		Price:        "0.00",
		Reason:       "no remaining capacity",
	}

	overlay := g.Generate(decision)

	if overlay != nil {
		t.Errorf("expected nil overlay for ShouldExist=false, got %+v", overlay)
	}
}

func TestGenerator_GenerateAll(t *testing.T) {
	g := NewGenerator()

	decisions := []Decision{
		{
			Name:         "cost-aware-compute-sp-global",
			CapacityType: CapacityTypeComputeSavingsPlan,
			ShouldExist:  true,
			Weight:       10,
			Price:        "0.00",
			Reason:       "capacity available",
		},
		{
			Name:         "cost-aware-ec2-sp-m5-us-west-2",
			CapacityType: CapacityTypeEC2InstanceSavingsPlan,
			ShouldExist:  false, // Should be marked for deletion
			Weight:       20,
			Price:        "0.00",
			Reason:       "no remaining capacity",
		},
		{
			Name:         "cost-aware-ri-c5.large-us-east-1",
			CapacityType: CapacityTypeReservedInstance,
			ShouldExist:  true,
			Weight:       30,
			Price:        "0.00",
			Reason:       "2 reserved instances available",
		},
	}

	results := g.GenerateAll(decisions)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// First result: should have overlay and action=create
	if results[0].Overlay == nil {
		t.Error("expected first result to have overlay")
	}
	if results[0].Action != "create" {
		t.Errorf("expected first result action %q, got %q", "create", results[0].Action)
	}

	// Second result: should have no overlay and action=delete
	if results[1].Overlay != nil {
		t.Error("expected second result to have nil overlay")
	}
	if results[1].Action != "delete" {
		t.Errorf("expected second result action %q, got %q", "delete", results[1].Action)
	}

	// Third result: should have overlay and action=create
	if results[2].Overlay == nil {
		t.Error("expected third result to have overlay")
	}
	if results[2].Action != "create" {
		t.Errorf("expected third result action %q, got %q", "create", results[2].Action)
	}
}

func TestParseEC2InstanceSPName(t *testing.T) {
	tests := []struct {
		name           string
		expectedFamily string
		expectedRegion string
	}{
		{
			name:           "cost-aware-ec2-sp-m5-us-west-2",
			expectedFamily: "m5",
			expectedRegion: "us-west-2",
		},
		{
			name:           "cost-aware-ec2-sp-c5-eu-central-1",
			expectedFamily: "c5",
			expectedRegion: "eu-central-1",
		},
		{
			name:           "cost-aware-ec2-sp-r6i-ap-southeast-1",
			expectedFamily: "r6i",
			expectedRegion: "ap-southeast-1",
		},
		{
			name:           "cost-aware-ec2-sp-m5a-us-east-1",
			expectedFamily: "m5a",
			expectedRegion: "us-east-1",
		},
		{
			name:           "invalid-prefix-m5-us-west-2",
			expectedFamily: "",
			expectedRegion: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			family, region := parseEC2InstanceSPName(tc.name)
			if family != tc.expectedFamily {
				t.Errorf("expected family %q, got %q", tc.expectedFamily, family)
			}
			if region != tc.expectedRegion {
				t.Errorf("expected region %q, got %q", tc.expectedRegion, region)
			}
		})
	}
}

func TestParseRIName(t *testing.T) {
	tests := []struct {
		name           string
		expectedType   string
		expectedRegion string
	}{
		{
			name:           "cost-aware-ri-m5.xlarge-us-west-2",
			expectedType:   "m5.xlarge",
			expectedRegion: "us-west-2",
		},
		{
			name:           "cost-aware-ri-c5.2xlarge-eu-central-1",
			expectedType:   "c5.2xlarge",
			expectedRegion: "eu-central-1",
		},
		{
			name:           "cost-aware-ri-r6i.large-ap-southeast-1",
			expectedType:   "r6i.large",
			expectedRegion: "ap-southeast-1",
		},
		{
			name:           "invalid-prefix-m5.xlarge-us-west-2",
			expectedType:   "",
			expectedRegion: "",
		},
		{
			name:           "cost-aware-ri-invalid-no-dot",
			expectedType:   "",
			expectedRegion: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			instanceType, region := parseRIName(tc.name)
			if instanceType != tc.expectedType {
				t.Errorf("expected instance type %q, got %q", tc.expectedType, instanceType)
			}
			if region != tc.expectedRegion {
				t.Errorf("expected region %q, got %q", tc.expectedRegion, region)
			}
		})
	}
}

func TestSanitizeLabelValue(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "simple",
			expected: "simple",
		},
		{
			input:    "with spaces",
			expected: "with-spaces",
		},
		{
			input:    "special!@#$%chars",
			expected: "special-chars",
		},
		{
			input:    "---leading-trailing---",
			expected: "leading-trailing",
		},
		{
			input:    strings.Repeat("a", 100),
			expected: strings.Repeat("a", 63),
		},
		{
			input:    "",
			expected: "capacity-available",
		},
		{
			input:    "utilization 50.0% below threshold 95.0%",
			expected: "utilization-50.0-below-threshold-95.0",
		},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := sanitizeLabelValue(tc.input)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestCapacityTypeToLabelValue(t *testing.T) {
	tests := []struct {
		input    CapacityType
		expected string
	}{
		{CapacityTypeComputeSavingsPlan, "compute-savings-plan"},
		{CapacityTypeEC2InstanceSavingsPlan, "ec2-instance-savings-plan"},
		{CapacityTypeReservedInstance, "reserved-instance"},
		{CapacityType("unknown"), "unknown"},
	}

	for _, tc := range tests {
		t.Run(string(tc.input), func(t *testing.T) {
			result := capacityTypeToLabelValue(tc.input)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestValidateOverlay(t *testing.T) {
	validPrice := "0.00"
	validWeight := int32(10)

	tests := []struct {
		name           string
		overlay        *karpenterv1alpha1.NodeOverlay
		expectedErrors int
		errorContains  []string
	}{
		{
			name:           "nil overlay",
			overlay:        nil,
			expectedErrors: 1,
			errorContains:  []string{"overlay is nil"},
		},
		{
			name: "valid overlay",
			overlay: &karpenterv1alpha1.NodeOverlay{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-overlay",
				},
				Spec: karpenterv1alpha1.NodeOverlaySpec{
					Requirements: []corev1.NodeSelectorRequirement{
						{
							Key:      LabelInstanceFamilyKarpenter,
							Operator: corev1.NodeSelectorOpExists,
						},
					},
					Price:  &validPrice,
					Weight: &validWeight,
				},
			},
			expectedErrors: 0,
		},
		{
			name: "missing name",
			overlay: &karpenterv1alpha1.NodeOverlay{
				Spec: karpenterv1alpha1.NodeOverlaySpec{
					Requirements: []corev1.NodeSelectorRequirement{
						{
							Key:      LabelInstanceFamilyKarpenter,
							Operator: corev1.NodeSelectorOpExists,
						},
					},
				},
			},
			expectedErrors: 1,
			errorContains:  []string{"name is required"},
		},
		{
			name: "missing requirements",
			overlay: &karpenterv1alpha1.NodeOverlay{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-overlay",
				},
				Spec: karpenterv1alpha1.NodeOverlaySpec{
					Price:  &validPrice,
					Weight: &validWeight,
				},
			},
			expectedErrors: 1,
			errorContains:  []string{"at least one requirement is required"},
		},
		{
			name: "In operator without values",
			overlay: &karpenterv1alpha1.NodeOverlay{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-overlay",
				},
				Spec: karpenterv1alpha1.NodeOverlaySpec{
					Requirements: []corev1.NodeSelectorRequirement{
						{
							Key:      LabelInstanceFamilyKarpenter,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{}, // Empty!
						},
					},
				},
			},
			expectedErrors: 1,
			errorContains:  []string{"requires at least one value"},
		},
		{
			name: "invalid price format",
			overlay: &karpenterv1alpha1.NodeOverlay{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-overlay",
				},
				Spec: karpenterv1alpha1.NodeOverlaySpec{
					Requirements: []corev1.NodeSelectorRequirement{
						{
							Key:      LabelInstanceFamilyKarpenter,
							Operator: corev1.NodeSelectorOpExists,
						},
					},
					Price: stringPtr("-1.00"), // Negative
				},
			},
			expectedErrors: 1,
			errorContains:  []string{"must be a non-negative decimal"},
		},
		{
			name: "weight out of range - too low",
			overlay: &karpenterv1alpha1.NodeOverlay{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-overlay",
				},
				Spec: karpenterv1alpha1.NodeOverlaySpec{
					Requirements: []corev1.NodeSelectorRequirement{
						{
							Key:      LabelInstanceFamilyKarpenter,
							Operator: corev1.NodeSelectorOpExists,
						},
					},
					Weight: int32Ptr(0),
				},
			},
			expectedErrors: 1,
			errorContains:  []string{"must be between 1 and 10000"},
		},
		{
			name: "weight out of range - too high",
			overlay: &karpenterv1alpha1.NodeOverlay{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-overlay",
				},
				Spec: karpenterv1alpha1.NodeOverlaySpec{
					Requirements: []corev1.NodeSelectorRequirement{
						{
							Key:      LabelInstanceFamilyKarpenter,
							Operator: corev1.NodeSelectorOpExists,
						},
					},
					Weight: int32Ptr(10001),
				},
			},
			expectedErrors: 1,
			errorContains:  []string{"must be between 1 and 10000"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errors := ValidateOverlay(tc.overlay)
			if len(errors) != tc.expectedErrors {
				t.Errorf("expected %d errors, got %d: %v", tc.expectedErrors, len(errors), errors)
			}

			for _, contains := range tc.errorContains {
				found := false
				for _, err := range errors {
					if strings.Contains(err.Error(), contains) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error containing %q, but not found in %v", contains, errors)
				}
			}
		})
	}
}

func TestFormatOverlayYAML(t *testing.T) {
	price := "0.00"
	weight := int32(10)

	overlay := &karpenterv1alpha1.NodeOverlay{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cost-aware-compute-sp-global",
			Labels: map[string]string{
				LabelManagedBy:    LabelManagedByValue,
				LabelCapacityType: "compute-savings-plan",
			},
		},
		Spec: karpenterv1alpha1.NodeOverlaySpec{
			Requirements: []corev1.NodeSelectorRequirement{
				{
					Key:      LabelInstanceFamilyKarpenter,
					Operator: corev1.NodeSelectorOpExists,
				},
				{
					Key:      LabelCapacityTypeKarpenter,
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"on-demand"},
				},
			},
			Price:  &price,
			Weight: &weight,
		},
	}

	yaml := FormatOverlayYAML(overlay)

	// Check that the YAML contains expected fields
	expectedFields := []string{
		"apiVersion: karpenter.sh/v1alpha1",
		"kind: NodeOverlay",
		"name: cost-aware-compute-sp-global",
		"weight: 10",
		`price: "0.00"`,
		"requirements:",
		"key: karpenter.k8s.aws/instance-family",
		"operator: Exists",
		"key: karpenter.sh/capacity-type",
		"operator: In",
	}

	for _, field := range expectedFields {
		if !strings.Contains(yaml, field) {
			t.Errorf("expected YAML to contain %q, got:\n%s", field, yaml)
		}
	}
}

func TestFormatOverlayYAML_Nil(t *testing.T) {
	result := FormatOverlayYAML(nil)
	if result != "" {
		t.Errorf("expected empty string for nil overlay, got %q", result)
	}
}

// stringPtr returns a pointer to a string value.
func stringPtr(s string) *string {
	return &s
}
