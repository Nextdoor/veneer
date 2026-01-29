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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	karpenterv1alpha1 "sigs.k8s.io/karpenter/pkg/apis/v1alpha1"
)

// Test constants for commonly used values
const (
	testNodePoolMyWorkload = "my-workload"
	testInstanceFamilyC7a  = "c7a"
	testInstanceFamilyC7g  = "c7g"
)

//nolint:gocyclo // Table-driven tests with inline assertions have high cyclomatic complexity
func TestGenerator_Generate(t *testing.T) {
	tests := []struct {
		name     string
		disabled bool
		pref     Preference
		check    func(t *testing.T, overlay *karpenterv1alpha1.NodeOverlay)
	}{
		{
			name:     "single matcher preference",
			disabled: false,
			pref: Preference{
				Number:       1,
				NodePoolName: testNodePoolMyWorkload,
				Adjustment:   -20,
				Matchers: []LabelMatcher{
					{
						Key:      LabelInstanceFamily,
						Operator: OperatorIn,
						Values:   []string{testInstanceFamilyC7a, testInstanceFamilyC7g},
					},
				},
			},
			check: func(t *testing.T, o *karpenterv1alpha1.NodeOverlay) {
				expectedName := "pref-" + testNodePoolMyWorkload + "-1"
				if o.Name != expectedName {
					t.Errorf("expected name %s, got %s", expectedName, o.Name)
				}

				// Check labels
				if o.Labels[LabelManagedBy] != LabelManagedByValue {
					t.Errorf("expected managed-by label %s, got %s", LabelManagedByValue, o.Labels[LabelManagedBy])
				}
				if o.Labels[LabelPreferenceType] != LabelPreferenceTypeValue {
					t.Errorf("expected type label %s, got %s", LabelPreferenceTypeValue, o.Labels[LabelPreferenceType])
				}
				if o.Labels[LabelSourceNodePool] != testNodePoolMyWorkload {
					t.Errorf("expected source-nodepool label, got %s", o.Labels[LabelSourceNodePool])
				}
				if o.Labels[LabelPreferenceNumber] != "1" {
					t.Errorf("expected preference-number label 1, got %s", o.Labels[LabelPreferenceNumber])
				}

				// Check weight
				if o.Spec.Weight == nil || *o.Spec.Weight != 1 {
					t.Errorf("expected weight 1, got %v", o.Spec.Weight)
				}

				// Check priceAdjustment
				if o.Spec.PriceAdjustment == nil || *o.Spec.PriceAdjustment != "-20%" {
					t.Errorf("expected priceAdjustment -20%%, got %v", o.Spec.PriceAdjustment)
				}

				// Check requirements (nodepool + matcher)
				if len(o.Spec.Requirements) != 2 {
					t.Errorf("expected 2 requirements, got %d", len(o.Spec.Requirements))
				}

				// First requirement should be nodepool scope
				npReq := o.Spec.Requirements[0]
				if npReq.Key != LabelNodePool {
					t.Errorf("expected first req key %s, got %s", LabelNodePool, npReq.Key)
				}
				if npReq.Operator != corev1.NodeSelectorOpIn {
					t.Errorf("expected first req operator In, got %s", npReq.Operator)
				}
				if len(npReq.Values) != 1 || npReq.Values[0] != testNodePoolMyWorkload {
					t.Errorf("expected first req values [%s], got %v", testNodePoolMyWorkload, npReq.Values)
				}

				// Second requirement should be the user matcher
				userReq := o.Spec.Requirements[1]
				if userReq.Key != LabelInstanceFamily {
					t.Errorf("expected second req key %s, got %s", LabelInstanceFamily, userReq.Key)
				}
				if userReq.Operator != corev1.NodeSelectorOpIn {
					t.Errorf("expected second req operator In, got %s", userReq.Operator)
				}
				expVals := []string{testInstanceFamilyC7a, testInstanceFamilyC7g}
				if len(userReq.Values) != 2 || userReq.Values[0] != expVals[0] || userReq.Values[1] != expVals[1] {
					t.Errorf("expected second req values %v, got %v", expVals, userReq.Values)
				}
			},
		},
		{
			name:     "disabled mode adds impossible requirement",
			disabled: true,
			pref: Preference{
				Number:       2,
				NodePoolName: "test-pool",
				Adjustment:   10,
				Matchers: []LabelMatcher{
					{
						Key:      LabelArch,
						Operator: OperatorIn,
						Values:   []string{"arm64"},
					},
				},
			},
			check: func(t *testing.T, o *karpenterv1alpha1.NodeOverlay) {
				// Check disabled label is set
				if o.Labels[LabelDisabledKey] != LabelDisabledValue {
					t.Errorf("expected disabled label %s, got %s", LabelDisabledValue, o.Labels[LabelDisabledKey])
				}

				// Check first requirement is the disabled requirement
				if len(o.Spec.Requirements) < 1 {
					t.Errorf("expected at least 1 requirement, got %d", len(o.Spec.Requirements))
					return
				}
				disabledReq := o.Spec.Requirements[0]
				if disabledReq.Key != LabelDisabledKey {
					t.Errorf("expected first req key %s, got %s", LabelDisabledKey, disabledReq.Key)
				}
				if len(disabledReq.Values) != 1 || disabledReq.Values[0] != LabelDisabledValue {
					t.Errorf("expected disabled req values [true], got %v", disabledReq.Values)
				}

				// Total requirements: disabled + nodepool + matcher = 3
				if len(o.Spec.Requirements) != 3 {
					t.Errorf("expected 3 requirements, got %d", len(o.Spec.Requirements))
				}
			},
		},
		{
			name:     "multiple matchers",
			disabled: false,
			pref: Preference{
				Number:       3,
				NodePoolName: "multi",
				Adjustment:   -30,
				Matchers: []LabelMatcher{
					{
						Key:      LabelInstanceFamily,
						Operator: OperatorIn,
						Values:   []string{"m7g"},
					},
					{
						Key:      LabelArch,
						Operator: OperatorIn,
						Values:   []string{"arm64"},
					},
				},
			},
			check: func(t *testing.T, o *karpenterv1alpha1.NodeOverlay) {
				// nodepool + 2 matchers = 3 requirements
				if len(o.Spec.Requirements) != 3 {
					t.Errorf("expected 3 requirements, got %d", len(o.Spec.Requirements))
				}
			},
		},
		{
			name:     "NotIn operator",
			disabled: false,
			pref: Preference{
				Number:       1,
				NodePoolName: "notin-test",
				Adjustment:   5,
				Matchers: []LabelMatcher{
					{
						Key:      LabelInstanceFamily,
						Operator: OperatorNotIn,
						Values:   []string{"m5", "m6i"},
					},
				},
			},
			check: func(t *testing.T, o *karpenterv1alpha1.NodeOverlay) {
				// Find the user matcher
				var userReq corev1.NodeSelectorRequirement
				for _, req := range o.Spec.Requirements {
					if req.Key == LabelInstanceFamily {
						userReq = req
						break
					}
				}
				if userReq.Operator != corev1.NodeSelectorOpNotIn {
					t.Errorf("expected NotIn operator, got %s", userReq.Operator)
				}
			},
		},
		{
			name:     "Gt operator",
			disabled: false,
			pref: Preference{
				Number:       1,
				NodePoolName: "gt-test",
				Adjustment:   -10,
				Matchers: []LabelMatcher{
					{
						Key:      LabelInstanceCPU,
						Operator: OperatorGt,
						Values:   []string{"4"},
					},
				},
			},
			check: func(t *testing.T, o *karpenterv1alpha1.NodeOverlay) {
				var userReq corev1.NodeSelectorRequirement
				for _, req := range o.Spec.Requirements {
					if req.Key == LabelInstanceCPU {
						userReq = req
						break
					}
				}
				if userReq.Operator != corev1.NodeSelectorOpGt {
					t.Errorf("expected Gt operator, got %s", userReq.Operator)
				}
				if len(userReq.Values) != 1 || userReq.Values[0] != "4" {
					t.Errorf("expected values [4], got %v", userReq.Values)
				}
			},
		},
		{
			name:     "Lt operator",
			disabled: false,
			pref: Preference{
				Number:       1,
				NodePoolName: "lt-test",
				Adjustment:   15,
				Matchers: []LabelMatcher{
					{
						Key:      LabelInstanceMemory,
						Operator: OperatorLt,
						Values:   []string{"16384"},
					},
				},
			},
			check: func(t *testing.T, o *karpenterv1alpha1.NodeOverlay) {
				var userReq corev1.NodeSelectorRequirement
				for _, req := range o.Spec.Requirements {
					if req.Key == LabelInstanceMemory {
						userReq = req
						break
					}
				}
				if userReq.Operator != corev1.NodeSelectorOpLt {
					t.Errorf("expected Lt operator, got %s", userReq.Operator)
				}
			},
		},
		{
			name:     "positive adjustment",
			disabled: false,
			pref: Preference{
				Number:       1,
				NodePoolName: "pos-adj",
				Adjustment:   40,
				Matchers: []LabelMatcher{
					{
						Key:      LabelArch,
						Operator: OperatorIn,
						Values:   []string{"amd64"},
					},
				},
			},
			check: func(t *testing.T, o *karpenterv1alpha1.NodeOverlay) {
				if o.Spec.PriceAdjustment == nil || *o.Spec.PriceAdjustment != "+40%" {
					t.Errorf("expected priceAdjustment +40%%, got %v", o.Spec.PriceAdjustment)
				}
			},
		},
		{
			name:     "zero adjustment",
			disabled: false,
			pref: Preference{
				Number:       1,
				NodePoolName: "zero-adj",
				Adjustment:   0,
				Matchers: []LabelMatcher{
					{
						Key:      LabelArch,
						Operator: OperatorIn,
						Values:   []string{"arm64"},
					},
				},
			},
			check: func(t *testing.T, o *karpenterv1alpha1.NodeOverlay) {
				if o.Spec.PriceAdjustment == nil || *o.Spec.PriceAdjustment != "0%" {
					t.Errorf("expected priceAdjustment 0%%, got %v", o.Spec.PriceAdjustment)
				}
			},
		},
		{
			name:     "high preference number",
			disabled: false,
			pref: Preference{
				Number:       100,
				NodePoolName: "high-num",
				Adjustment:   -5,
				Matchers: []LabelMatcher{
					{
						Key:      LabelCapacityType,
						Operator: OperatorIn,
						Values:   []string{"spot"},
					},
				},
			},
			check: func(t *testing.T, o *karpenterv1alpha1.NodeOverlay) {
				if o.Name != "pref-high-num-100" {
					t.Errorf("expected name pref-high-num-100, got %s", o.Name)
				}
				if o.Spec.Weight == nil || *o.Spec.Weight != 100 {
					t.Errorf("expected weight 100, got %v", o.Spec.Weight)
				}
				if o.Labels[LabelPreferenceNumber] != "100" {
					t.Errorf("expected preference-number label 100, got %s", o.Labels[LabelPreferenceNumber])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGeneratorWithOptions(tt.disabled)
			overlay := g.Generate(tt.pref)

			if overlay == nil {
				t.Fatal("expected non-nil overlay")
			}

			if overlay.TypeMeta.APIVersion != "karpenter.sh/v1alpha1" {
				t.Errorf("expected APIVersion karpenter.sh/v1alpha1, got %s", overlay.TypeMeta.APIVersion)
			}
			if overlay.TypeMeta.Kind != "NodeOverlay" {
				t.Errorf("expected Kind NodeOverlay, got %s", overlay.TypeMeta.Kind)
			}

			tt.check(t, overlay)
		})
	}
}

func TestGenerator_GenerateAll(t *testing.T) {
	g := NewGenerator()

	prefs := []Preference{
		{
			Number:       1,
			NodePoolName: "pool-a",
			Adjustment:   -10,
			Matchers: []LabelMatcher{
				{Key: LabelInstanceFamily, Operator: OperatorIn, Values: []string{"c7a"}},
			},
		},
		{
			Number:       2,
			NodePoolName: "pool-a",
			Adjustment:   -20,
			Matchers: []LabelMatcher{
				{Key: LabelArch, Operator: OperatorIn, Values: []string{"arm64"}},
			},
		},
		{
			Number:       3,
			NodePoolName: "pool-b",
			Adjustment:   5,
			Matchers: []LabelMatcher{
				{Key: LabelCapacityType, Operator: OperatorIn, Values: []string{"spot"}},
			},
		},
	}

	overlays := g.GenerateAll(prefs)

	if len(overlays) != 3 {
		t.Errorf("expected 3 overlays, got %d", len(overlays))
	}

	// Verify each overlay corresponds to the correct preference
	expectedNames := []string{"pref-pool-a-1", "pref-pool-a-2", "pref-pool-b-3"}
	for i, overlay := range overlays {
		if overlay.Name != expectedNames[i] {
			t.Errorf("overlay %d: expected name %s, got %s", i, expectedNames[i], overlay.Name)
		}
	}
}

func TestOverlayNameForPreference(t *testing.T) {
	tests := []struct {
		nodePoolName string
		prefNum      int
		want         string
	}{
		{"my-workload", 1, "pref-my-workload-1"},
		{"test-pool", 10, "pref-test-pool-10"},
		{"a", 999, "pref-a-999"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := OverlayNameForPreference(tt.nodePoolName, tt.prefNum)
			if got != tt.want {
				t.Errorf("expected %s, got %s", tt.want, got)
			}
		})
	}
}

func TestIsPreferenceOverlay(t *testing.T) {
	tests := []struct {
		name    string
		overlay *karpenterv1alpha1.NodeOverlay
		want    bool
	}{
		{
			name:    "nil overlay",
			overlay: nil,
			want:    false,
		},
		{
			name: "nil labels",
			overlay: &karpenterv1alpha1.NodeOverlay{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			},
			want: false,
		},
		{
			name: "missing managed-by label",
			overlay: &karpenterv1alpha1.NodeOverlay{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Labels: map[string]string{
						LabelPreferenceType: LabelPreferenceTypeValue,
					},
				},
			},
			want: false,
		},
		{
			name: "missing type label",
			overlay: &karpenterv1alpha1.NodeOverlay{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Labels: map[string]string{
						LabelManagedBy: LabelManagedByValue,
					},
				},
			},
			want: false,
		},
		{
			name: "wrong type label",
			overlay: &karpenterv1alpha1.NodeOverlay{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Labels: map[string]string{
						LabelManagedBy:      LabelManagedByValue,
						LabelPreferenceType: "cost",
					},
				},
			},
			want: false,
		},
		{
			name: "valid preference overlay",
			overlay: &karpenterv1alpha1.NodeOverlay{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pref-pool-1",
					Labels: map[string]string{
						LabelManagedBy:      LabelManagedByValue,
						LabelPreferenceType: LabelPreferenceTypeValue,
					},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsPreferenceOverlay(tt.overlay)
			if got != tt.want {
				t.Errorf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

func TestGetSourceNodePool(t *testing.T) {
	tests := []struct {
		name    string
		overlay *karpenterv1alpha1.NodeOverlay
		want    string
	}{
		{
			name:    "nil overlay",
			overlay: nil,
			want:    "",
		},
		{
			name: "with source label",
			overlay: &karpenterv1alpha1.NodeOverlay{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						LabelSourceNodePool: "my-pool",
					},
				},
			},
			want: "my-pool",
		},
		{
			name: "without source label",
			overlay: &karpenterv1alpha1.NodeOverlay{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{},
				},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetSourceNodePool(tt.overlay)
			if got != tt.want {
				t.Errorf("expected %s, got %s", tt.want, got)
			}
		})
	}
}

func TestGetPreferenceNumber(t *testing.T) {
	tests := []struct {
		name    string
		overlay *karpenterv1alpha1.NodeOverlay
		want    int
	}{
		{
			name:    "nil overlay",
			overlay: nil,
			want:    0,
		},
		{
			name: "with valid number",
			overlay: &karpenterv1alpha1.NodeOverlay{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						LabelPreferenceNumber: "5",
					},
				},
			},
			want: 5,
		},
		{
			name: "with invalid number",
			overlay: &karpenterv1alpha1.NodeOverlay{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						LabelPreferenceNumber: "abc",
					},
				},
			},
			want: 0,
		},
		{
			name: "without number label",
			overlay: &karpenterv1alpha1.NodeOverlay{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{},
				},
			},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetPreferenceNumber(tt.overlay)
			if got != tt.want {
				t.Errorf("expected %d, got %d", tt.want, got)
			}
		})
	}
}
