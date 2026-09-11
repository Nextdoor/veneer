// Copyright 2025 Nextdoor, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package reconciler

import (
	"context"
	"strings"
	"testing"

	"github.com/awslabs/operatorpkg/status"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	veneermetrics "github.com/nextdoor/veneer/pkg/metrics"
	"github.com/nextdoor/veneer/pkg/preference"
	promclient "github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	karpenterv1alpha1 "sigs.k8s.io/karpenter/pkg/apis/v1alpha1"
)

func setupTestScheme(t *testing.T) *runtime.Scheme {
	scheme := runtime.NewScheme()

	// Add core types
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add core types to scheme: %v", err)
	}

	// Add Karpenter v1 types (NodePool)
	// Karpenter doesn't export SchemeGroupVersion, so we define it manually
	karpenterv1GV := schema.GroupVersion{Group: "karpenter.sh", Version: "v1"}
	scheme.AddKnownTypes(karpenterv1GV, &karpenterv1.NodePool{}, &karpenterv1.NodePoolList{})
	metav1.AddToGroupVersion(scheme, karpenterv1GV)

	// Add Karpenter v1alpha1 types (NodeOverlay)
	karpenterv1alpha1GV := schema.GroupVersion{Group: "karpenter.sh", Version: "v1alpha1"}
	scheme.AddKnownTypes(karpenterv1alpha1GV, &karpenterv1alpha1.NodeOverlay{}, &karpenterv1alpha1.NodeOverlayList{})
	metav1.AddToGroupVersion(scheme, karpenterv1alpha1GV)

	return scheme
}

func TestNodePoolReconciler_Reconcile_CreateOverlays(t *testing.T) {
	scheme := setupTestScheme(t)

	// Create a NodePool with preference annotations
	nodePool := &karpenterv1.NodePool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pool",
			Annotations: map[string]string{
				"veneer.io/preference.1": "karpenter.k8s.aws/instance-family=c7a,c7g adjust=-20%",
				"veneer.io/preference.2": "kubernetes.io/arch=arm64 adjust=+10%",
			},
		},
		Spec: karpenterv1.NodePoolSpec{},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(nodePool).
		Build()

	reconciler := &NodePoolReconciler{
		Client:    client,
		Logger:    logr.Discard(),
		Generator: preference.NewGenerator(),
	}

	// Reconcile
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-pool"}}
	result, err := reconciler.Reconcile(context.Background(), req)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Requeue {
		t.Errorf("unexpected requeue")
	}

	// Verify overlays were created
	var overlayList karpenterv1alpha1.NodeOverlayList
	if err := client.List(context.Background(), &overlayList); err != nil {
		t.Fatalf("failed to list overlays: %v", err)
	}

	if len(overlayList.Items) != 2 {
		t.Errorf("expected 2 overlays, got %d", len(overlayList.Items))
	}

	// Verify overlay names
	expectedNames := map[string]bool{
		"pref-test-pool-1": false,
		"pref-test-pool-2": false,
	}
	for _, overlay := range overlayList.Items {
		if _, ok := expectedNames[overlay.Name]; !ok {
			t.Errorf("unexpected overlay name: %s", overlay.Name)
		}
		expectedNames[overlay.Name] = true

		// Verify labels
		if overlay.Labels[preference.LabelManagedBy] != preference.LabelManagedByValue {
			t.Errorf("overlay %s: expected managed-by label %s, got %s",
				overlay.Name, preference.LabelManagedByValue, overlay.Labels[preference.LabelManagedBy])
		}
		if overlay.Labels[preference.LabelPreferenceType] != preference.LabelPreferenceTypeValue {
			t.Errorf("overlay %s: expected type label %s, got %s",
				overlay.Name, preference.LabelPreferenceTypeValue, overlay.Labels[preference.LabelPreferenceType])
		}
		if overlay.Labels[preference.LabelSourceNodePool] != "test-pool" {
			t.Errorf("overlay %s: expected source-nodepool label test-pool, got %s",
				overlay.Name, overlay.Labels[preference.LabelSourceNodePool])
		}
	}

	for name, found := range expectedNames {
		if !found {
			t.Errorf("expected overlay %s was not created", name)
		}
	}
}

func TestNodePoolReconciler_Reconcile_UpdateOverlays(t *testing.T) {
	scheme := setupTestScheme(t)

	// Create a NodePool with preference annotations
	nodePool := &karpenterv1.NodePool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "update-pool",
			Annotations: map[string]string{
				"veneer.io/preference.1": "karpenter.k8s.aws/instance-family=m7g adjust=-30%",
			},
		},
		Spec: karpenterv1.NodePoolSpec{},
	}

	// Create an existing overlay with old spec
	existingOverlay := &karpenterv1alpha1.NodeOverlay{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pref-update-pool-1",
			Labels: map[string]string{
				preference.LabelManagedBy:        preference.LabelManagedByValue,
				preference.LabelPreferenceType:   preference.LabelPreferenceTypeValue,
				preference.LabelSourceNodePool:   "update-pool",
				preference.LabelPreferenceNumber: "1",
			},
		},
		Spec: karpenterv1alpha1.NodeOverlaySpec{
			PriceAdjustment: strPtr("-10%"), // Old value
			Weight:          int32Ptr(1),
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(nodePool, existingOverlay).
		Build()

	reconciler := &NodePoolReconciler{
		Client:    client,
		Logger:    logr.Discard(),
		Generator: preference.NewGenerator(),
	}

	// Reconcile
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "update-pool"}}
	_, err := reconciler.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify overlay was updated
	var overlay karpenterv1alpha1.NodeOverlay
	if err := client.Get(context.Background(), types.NamespacedName{Name: "pref-update-pool-1"}, &overlay); err != nil {
		t.Fatalf("failed to get overlay: %v", err)
	}

	if overlay.Spec.PriceAdjustment == nil || *overlay.Spec.PriceAdjustment != "-30%" {
		t.Errorf("expected priceAdjustment -30%%, got %v", overlay.Spec.PriceAdjustment)
	}
}

func TestNodePoolReconciler_Reconcile_DeleteStaleOverlays(t *testing.T) {
	scheme := setupTestScheme(t)

	// Create a NodePool with only one preference (removed preference.2)
	nodePool := &karpenterv1.NodePool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "delete-pool",
			Annotations: map[string]string{
				"veneer.io/preference.1": "karpenter.k8s.aws/instance-family=c7a adjust=-20%",
			},
		},
		Spec: karpenterv1.NodePoolSpec{},
	}

	// Create two existing overlays (one is now stale)
	overlay1 := &karpenterv1alpha1.NodeOverlay{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pref-delete-pool-1",
			Labels: map[string]string{
				preference.LabelManagedBy:        preference.LabelManagedByValue,
				preference.LabelPreferenceType:   preference.LabelPreferenceTypeValue,
				preference.LabelSourceNodePool:   "delete-pool",
				preference.LabelPreferenceNumber: "1",
			},
		},
	}
	overlay2 := &karpenterv1alpha1.NodeOverlay{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pref-delete-pool-2",
			Labels: map[string]string{
				preference.LabelManagedBy:        preference.LabelManagedByValue,
				preference.LabelPreferenceType:   preference.LabelPreferenceTypeValue,
				preference.LabelSourceNodePool:   "delete-pool",
				preference.LabelPreferenceNumber: "2",
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(nodePool, overlay1, overlay2).
		Build()

	reconciler := &NodePoolReconciler{
		Client:    client,
		Logger:    logr.Discard(),
		Generator: preference.NewGenerator(),
	}

	// Reconcile
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "delete-pool"}}
	_, err := reconciler.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify only one overlay remains
	var overlayList karpenterv1alpha1.NodeOverlayList
	if err := client.List(context.Background(), &overlayList); err != nil {
		t.Fatalf("failed to list overlays: %v", err)
	}

	if len(overlayList.Items) != 1 {
		t.Errorf("expected 1 overlay, got %d", len(overlayList.Items))
	}

	if len(overlayList.Items) > 0 && overlayList.Items[0].Name != "pref-delete-pool-1" {
		t.Errorf("expected remaining overlay to be pref-delete-pool-1, got %s", overlayList.Items[0].Name)
	}
}

func TestNodePoolReconciler_Reconcile_DeletedNodePool(t *testing.T) {
	scheme := setupTestScheme(t)

	// Create overlays that belonged to a now-deleted NodePool
	overlay1 := &karpenterv1alpha1.NodeOverlay{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pref-deleted-pool-1",
			Labels: map[string]string{
				preference.LabelManagedBy:        preference.LabelManagedByValue,
				preference.LabelPreferenceType:   preference.LabelPreferenceTypeValue,
				preference.LabelSourceNodePool:   "deleted-pool",
				preference.LabelPreferenceNumber: "1",
			},
		},
	}
	overlay2 := &karpenterv1alpha1.NodeOverlay{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pref-deleted-pool-2",
			Labels: map[string]string{
				preference.LabelManagedBy:        preference.LabelManagedByValue,
				preference.LabelPreferenceType:   preference.LabelPreferenceTypeValue,
				preference.LabelSourceNodePool:   "deleted-pool",
				preference.LabelPreferenceNumber: "2",
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(overlay1, overlay2).
		Build()

	reconciler := &NodePoolReconciler{
		Client:    client,
		Logger:    logr.Discard(),
		Generator: preference.NewGenerator(),
	}

	// Reconcile for deleted NodePool
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "deleted-pool"}}
	_, err := reconciler.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify all overlays were cleaned up
	var overlayList karpenterv1alpha1.NodeOverlayList
	if err := client.List(context.Background(), &overlayList); err != nil {
		t.Fatalf("failed to list overlays: %v", err)
	}

	if len(overlayList.Items) != 0 {
		t.Errorf("expected 0 overlays after cleanup, got %d", len(overlayList.Items))
	}
}

func TestNodePoolReconciler_Reconcile_NoPreferences(t *testing.T) {
	scheme := setupTestScheme(t)

	// Create a NodePool without preference annotations
	nodePool := &karpenterv1.NodePool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "no-pref-pool",
			Annotations: map[string]string{
				"some-other-annotation": "value",
			},
		},
		Spec: karpenterv1.NodePoolSpec{},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(nodePool).
		Build()

	reconciler := &NodePoolReconciler{
		Client:    client,
		Logger:    logr.Discard(),
		Generator: preference.NewGenerator(),
	}

	// Reconcile
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "no-pref-pool"}}
	_, err := reconciler.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify no overlays were created
	var overlayList karpenterv1alpha1.NodeOverlayList
	if err := client.List(context.Background(), &overlayList); err != nil {
		t.Fatalf("failed to list overlays: %v", err)
	}

	if len(overlayList.Items) != 0 {
		t.Errorf("expected 0 overlays, got %d", len(overlayList.Items))
	}
}

func TestNodePoolReconciler_Reconcile_InvalidPreference(t *testing.T) {
	scheme := setupTestScheme(t)

	// Create a NodePool with one valid and one invalid preference
	nodePool := &karpenterv1.NodePool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "invalid-pref-pool",
			Annotations: map[string]string{
				"veneer.io/preference.1": "karpenter.k8s.aws/instance-family=c7a adjust=-20%",
				"veneer.io/preference.2": "invalid-format-no-adjust",
			},
		},
		Spec: karpenterv1.NodePoolSpec{},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(nodePool).
		Build()

	reconciler := &NodePoolReconciler{
		Client:    client,
		Logger:    logr.Discard(),
		Generator: preference.NewGenerator(),
	}

	// Reconcile
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "invalid-pref-pool"}}
	_, err := reconciler.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify only valid preference created an overlay
	var overlayList karpenterv1alpha1.NodeOverlayList
	if err := client.List(context.Background(), &overlayList); err != nil {
		t.Fatalf("failed to list overlays: %v", err)
	}

	if len(overlayList.Items) != 1 {
		t.Errorf("expected 1 overlay (from valid preference), got %d", len(overlayList.Items))
	}

	if len(overlayList.Items) > 0 && overlayList.Items[0].Name != "pref-invalid-pref-pool-1" {
		t.Errorf("expected overlay pref-invalid-pref-pool-1, got %s", overlayList.Items[0].Name)
	}
}

func TestNodePoolReconciler_Reconcile_DisabledMode(t *testing.T) {
	scheme := setupTestScheme(t)

	// Create a NodePool with preference annotations
	nodePool := &karpenterv1.NodePool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "disabled-pool",
			Annotations: map[string]string{
				"veneer.io/preference.1": "karpenter.k8s.aws/instance-family=c7a adjust=-20%",
			},
		},
		Spec: karpenterv1.NodePoolSpec{},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(nodePool).
		Build()

	// Use generator with disabled mode
	reconciler := &NodePoolReconciler{
		Client:    client,
		Logger:    logr.Discard(),
		Generator: preference.NewGeneratorWithOptions(true), // Disabled mode
	}

	// Reconcile
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "disabled-pool"}}
	_, err := reconciler.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify overlay was created with disabled label
	var overlayList karpenterv1alpha1.NodeOverlayList
	if err := client.List(context.Background(), &overlayList); err != nil {
		t.Fatalf("failed to list overlays: %v", err)
	}

	if len(overlayList.Items) != 1 {
		t.Fatalf("expected 1 overlay, got %d", len(overlayList.Items))
	}

	overlay := overlayList.Items[0]
	if overlay.Labels[preference.LabelDisabledKey] != preference.LabelDisabledValue {
		t.Errorf("expected disabled label %s, got %s",
			preference.LabelDisabledValue, overlay.Labels[preference.LabelDisabledKey])
	}

	// Verify impossible requirement was added
	hasDisabledReq := false
	for _, req := range overlay.Spec.Requirements {
		if req.Key == preference.LabelDisabledKey {
			hasDisabledReq = true
			break
		}
	}
	if !hasDisabledReq {
		t.Error("expected disabled requirement in overlay spec")
	}
}

func TestNodePoolReconciler_Reconcile_OwnerReferences(t *testing.T) {
	scheme := setupTestScheme(t)

	// Create a NodePool with UID (required for owner reference)
	nodePool := &karpenterv1.NodePool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "owner-test-pool",
			UID:  types.UID("test-nodepool-uid-12345"),
			Annotations: map[string]string{
				"veneer.io/preference.1": "karpenter.k8s.aws/instance-family=c7a adjust=-20%",
			},
		},
		Spec: karpenterv1.NodePoolSpec{},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(nodePool).
		Build()

	reconciler := &NodePoolReconciler{
		Client:    client,
		Logger:    logr.Discard(),
		Generator: preference.NewGenerator(),
	}

	// Reconcile
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "owner-test-pool"}}
	_, err := reconciler.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify overlay was created with owner references
	var overlayList karpenterv1alpha1.NodeOverlayList
	if err := client.List(context.Background(), &overlayList); err != nil {
		t.Fatalf("failed to list overlays: %v", err)
	}

	if len(overlayList.Items) != 1 {
		t.Fatalf("expected 1 overlay, got %d", len(overlayList.Items))
	}

	overlay := overlayList.Items[0]

	// Verify NodePool owner reference is set.
	// Note: We only set NodePool as owner (not the controller Deployment) because
	// Kubernetes OwnerReferences don't support cross-scope ownership. NodeOverlays
	// are cluster-scoped, but Deployments are namespace-scoped.
	if len(overlay.OwnerReferences) != 1 {
		t.Errorf("expected 1 owner reference, got %d", len(overlay.OwnerReferences))
	}

	if len(overlay.OwnerReferences) > 0 {
		ref := overlay.OwnerReferences[0]
		if ref.Controller == nil || !*ref.Controller {
			t.Error("expected NodePool owner reference to be marked as controller owner")
		}
		if ref.Kind != "NodePool" {
			t.Errorf("expected owner reference kind NodePool, got %s", ref.Kind)
		}
		if ref.Name != "owner-test-pool" {
			t.Errorf("expected owner reference name owner-test-pool, got %s", ref.Name)
		}
		if ref.UID != nodePool.UID {
			t.Errorf("NodePool owner reference has wrong UID: got %s, want %s", ref.UID, nodePool.UID)
		}
		if ref.APIVersion != "karpenter.sh/v1" {
			t.Errorf("NodePool owner reference has wrong APIVersion: got %s, want karpenter.sh/v1", ref.APIVersion)
		}
	}
}

func TestOverlayNeedsUpdate(t *testing.T) {
	tests := []struct {
		name     string
		existing *karpenterv1alpha1.NodeOverlay
		desired  *karpenterv1alpha1.NodeOverlay
		want     bool
	}{
		{
			name: "identical overlays should not need update",
			existing: &karpenterv1alpha1.NodeOverlay{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pref-test-1",
					Labels: map[string]string{
						"managed-by":                  "veneer",
						"veneer.io/type":              "preference",
						"veneer.io/source-nodepool":   "test",
						"veneer.io/preference-number": "1",
					},
				},
				Spec: karpenterv1alpha1.NodeOverlaySpec{
					Requirements: []karpenterv1alpha1.NodeSelectorRequirement{
						{
							Key:      "karpenter.sh/nodepool",
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"test"},
						},
					},
					PriceAdjustment: strPtr("-20%"),
					Weight:          int32Ptr(1),
				},
			},
			desired: &karpenterv1alpha1.NodeOverlay{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pref-test-1",
					Labels: map[string]string{
						"managed-by":                  "veneer",
						"veneer.io/type":              "preference",
						"veneer.io/source-nodepool":   "test",
						"veneer.io/preference-number": "1",
					},
				},
				Spec: karpenterv1alpha1.NodeOverlaySpec{
					Requirements: []karpenterv1alpha1.NodeSelectorRequirement{
						{
							Key:      "karpenter.sh/nodepool",
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"test"},
						},
					},
					PriceAdjustment: strPtr("-20%"),
					Weight:          int32Ptr(1),
				},
			},
			want: false,
		},
		{
			name: "different price adjustment should need update",
			existing: &karpenterv1alpha1.NodeOverlay{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "pref-test-1",
					Labels: map[string]string{"managed-by": "veneer"},
				},
				Spec: karpenterv1alpha1.NodeOverlaySpec{
					PriceAdjustment: strPtr("-20%"),
					Weight:          int32Ptr(1),
				},
			},
			desired: &karpenterv1alpha1.NodeOverlay{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "pref-test-1",
					Labels: map[string]string{"managed-by": "veneer"},
				},
				Spec: karpenterv1alpha1.NodeOverlaySpec{
					PriceAdjustment: strPtr("-30%"),
					Weight:          int32Ptr(1),
				},
			},
			want: true,
		},
		{
			name: "different weight should need update",
			existing: &karpenterv1alpha1.NodeOverlay{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "pref-test-1",
					Labels: map[string]string{"managed-by": "veneer"},
				},
				Spec: karpenterv1alpha1.NodeOverlaySpec{
					PriceAdjustment: strPtr("-20%"),
					Weight:          int32Ptr(1),
				},
			},
			desired: &karpenterv1alpha1.NodeOverlay{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "pref-test-1",
					Labels: map[string]string{"managed-by": "veneer"},
				},
				Spec: karpenterv1alpha1.NodeOverlaySpec{
					PriceAdjustment: strPtr("-20%"),
					Weight:          int32Ptr(2),
				},
			},
			want: true,
		},
		{
			name: "different requirements should need update",
			existing: &karpenterv1alpha1.NodeOverlay{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "pref-test-1",
					Labels: map[string]string{"managed-by": "veneer"},
				},
				Spec: karpenterv1alpha1.NodeOverlaySpec{
					Requirements: []karpenterv1alpha1.NodeSelectorRequirement{
						{Key: "foo", Operator: corev1.NodeSelectorOpIn, Values: []string{"bar"}},
					},
					PriceAdjustment: strPtr("-20%"),
				},
			},
			desired: &karpenterv1alpha1.NodeOverlay{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "pref-test-1",
					Labels: map[string]string{"managed-by": "veneer"},
				},
				Spec: karpenterv1alpha1.NodeOverlaySpec{
					Requirements: []karpenterv1alpha1.NodeSelectorRequirement{
						{Key: "foo", Operator: corev1.NodeSelectorOpIn, Values: []string{"baz"}},
					},
					PriceAdjustment: strPtr("-20%"),
				},
			},
			want: true,
		},
		{
			name: "missing label should need update",
			existing: &karpenterv1alpha1.NodeOverlay{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "pref-test-1",
					Labels: map[string]string{"managed-by": "veneer"},
				},
				Spec: karpenterv1alpha1.NodeOverlaySpec{
					PriceAdjustment: strPtr("-20%"),
				},
			},
			desired: &karpenterv1alpha1.NodeOverlay{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pref-test-1",
					Labels: map[string]string{
						"managed-by":     "veneer",
						"veneer.io/type": "preference",
					},
				},
				Spec: karpenterv1alpha1.NodeOverlaySpec{
					PriceAdjustment: strPtr("-20%"),
				},
			},
			want: true,
		},
		{
			name: "extra labels on existing should not need update",
			existing: &karpenterv1alpha1.NodeOverlay{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pref-test-1",
					Labels: map[string]string{
						"managed-by":  "veneer",
						"extra-label": "extra-value",
					},
				},
				Spec: karpenterv1alpha1.NodeOverlaySpec{
					PriceAdjustment: strPtr("-20%"),
				},
			},
			desired: &karpenterv1alpha1.NodeOverlay{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "pref-test-1",
					Labels: map[string]string{"managed-by": "veneer"},
				},
				Spec: karpenterv1alpha1.NodeOverlaySpec{
					PriceAdjustment: strPtr("-20%"),
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := overlayNeedsUpdate(tt.existing, tt.desired)
			if got != tt.want {
				t.Errorf("overlayNeedsUpdate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNodePoolReconciler_Reconcile_SkipsUpdateWhenUnchanged(t *testing.T) {
	scheme := setupTestScheme(t)

	nodePool := &karpenterv1.NodePool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "skip-update-pool",
			UID:  "test-uid-123",
			Annotations: map[string]string{
				"veneer.io/preference.1": "karpenter.k8s.aws/instance-family=c7a adjust=-20%",
			},
		},
		Spec: karpenterv1.NodePoolSpec{},
	}

	// Pre-create an overlay that already matches what the reconciler would generate
	existingOverlay := &karpenterv1alpha1.NodeOverlay{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pref-skip-update-pool-1",
			Labels: map[string]string{
				preference.LabelManagedBy:        preference.LabelManagedByValue,
				preference.LabelPreferenceType:   preference.LabelPreferenceTypeValue,
				preference.LabelSourceNodePool:   "skip-update-pool",
				preference.LabelPreferenceNumber: "1",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "karpenter.sh/v1",
					Kind:       "NodePool",
					Name:       "skip-update-pool",
					UID:        "test-uid-123",
				},
			},
			ResourceVersion: "12345",
		},
		Spec: karpenterv1alpha1.NodeOverlaySpec{
			Requirements: []karpenterv1alpha1.NodeSelectorRequirement{
				{
					Key:      "karpenter.sh/nodepool",
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"skip-update-pool"},
				},
				{
					Key:      "karpenter.k8s.aws/instance-family",
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"c7a"},
				},
			},
			PriceAdjustment: strPtr("-20%"),
			Weight:          int32Ptr(1),
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(nodePool, existingOverlay).
		Build()

	reconciler := &NodePoolReconciler{
		Client:    client,
		Logger:    logr.Discard(),
		Generator: preference.NewGenerator(),
	}

	// Reconcile - should detect overlay is already up to date
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "skip-update-pool"}}
	result, err := reconciler.Reconcile(context.Background(), req)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Requeue {
		t.Errorf("unexpected requeue")
	}

	// Verify the existing overlay was migrated to a controller owner reference
	// so status-only events can be routed through SetupWithManager's Owns watch.
	var overlayList karpenterv1alpha1.NodeOverlayList
	if err := client.List(context.Background(), &overlayList); err != nil {
		t.Fatalf("failed to list overlays: %v", err)
	}

	if len(overlayList.Items) != 1 {
		t.Fatalf("expected 1 overlay, got %d", len(overlayList.Items))
	}

	if ref := overlayList.Items[0].OwnerReferences[0]; ref.Controller == nil || !*ref.Controller {
		t.Error("expected existing overlay to be migrated to a controller owner reference")
	}
}

func TestFailedNodeOverlayCondition(t *testing.T) {
	tests := []struct {
		name       string
		conditions []status.Condition
		wantType   string
	}{
		{name: "no conditions yet"},
		{
			name: "validation failed",
			conditions: []status.Condition{{
				Type:    karpenterv1alpha1.ConditionTypeValidationSucceeded,
				Status:  metav1.ConditionFalse,
				Reason:  "InvalidRequirement",
				Message: "requirement key is not supported",
			}},
			wantType: karpenterv1alpha1.ConditionTypeValidationSucceeded,
		},
		{
			name: "ready failed",
			conditions: []status.Condition{{
				Type:    status.ConditionReady,
				Status:  metav1.ConditionFalse,
				Reason:  "UnhealthyDependents",
				Message: "validation has not succeeded",
			}},
			wantType: status.ConditionReady,
		},
		{
			name: "validation failure preferred over derived ready failure",
			conditions: []status.Condition{
				{Type: status.ConditionReady, Status: metav1.ConditionFalse},
				{Type: karpenterv1alpha1.ConditionTypeValidationSucceeded, Status: metav1.ConditionFalse},
			},
			wantType: karpenterv1alpha1.ConditionTypeValidationSucceeded,
		},
		{
			name: "all conditions true",
			conditions: []status.Condition{
				{Type: karpenterv1alpha1.ConditionTypeValidationSucceeded, Status: metav1.ConditionTrue},
				{Type: status.ConditionReady, Status: metav1.ConditionTrue},
			},
		},
		{
			name: "unknown is not failed",
			conditions: []status.Condition{
				{Type: karpenterv1alpha1.ConditionTypeValidationSucceeded, Status: metav1.ConditionUnknown},
				{Type: status.ConditionReady, Status: metav1.ConditionUnknown},
			},
		},
		{
			name: "stale failure is not reported",
			conditions: []status.Condition{{
				Type:               karpenterv1alpha1.ConditionTypeValidationSucceeded,
				Status:             metav1.ConditionFalse,
				ObservedGeneration: 1,
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			overlay := &karpenterv1alpha1.NodeOverlay{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Status:     karpenterv1alpha1.NodeOverlayStatus{Conditions: tt.conditions},
			}

			got := failedNodeOverlayCondition(overlay)
			if tt.wantType == "" {
				if got != nil {
					t.Fatalf("expected no failed condition, got %s", got.Type)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected failed condition %s, got nil", tt.wantType)
			}
			if got.Type != tt.wantType {
				t.Fatalf("expected failed condition %s, got %s", tt.wantType, got.Type)
			}
		})
	}
}

func TestNodePoolReconciler_Reconcile_ReportsRejectedOverlayAndRecovery(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	nodePool := &karpenterv1.NodePool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "status-pool",
			UID:  types.UID("status-pool-uid"),
			Annotations: map[string]string{
				"veneer.io/preference.1": "karpenter.k8s.aws/instance-family=c7a adjust=-20%",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&karpenterv1alpha1.NodeOverlay{}).
		WithObjects(nodePool).
		Build()

	reg := promclient.NewRegistry()
	m := veneermetrics.NewMetrics(reg)
	var logs []string
	logger := funcr.NewJSON(func(entry string) {
		logs = append(logs, entry)
	}, funcr.Options{})

	reconciler := &NodePoolReconciler{
		Client:    fakeClient,
		Logger:    logger,
		Generator: preference.NewGenerator(),
		Metrics:   m,
	}
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: nodePool.Name}}

	if _, err := reconciler.Reconcile(ctx, req); err != nil {
		t.Fatalf("unexpected initial reconcile error: %v", err)
	}

	var overlay karpenterv1alpha1.NodeOverlay
	key := types.NamespacedName{Name: "pref-status-pool-1"}
	if err := fakeClient.Get(ctx, key, &overlay); err != nil {
		t.Fatalf("failed to get created overlay: %v", err)
	}
	overlay.Status.Conditions = []status.Condition{
		{
			Type:    karpenterv1alpha1.ConditionTypeValidationSucceeded,
			Status:  metav1.ConditionFalse,
			Reason:  "InvalidRequirement",
			Message: "requirement key is not supported",
		},
		{
			Type:    status.ConditionReady,
			Status:  metav1.ConditionFalse,
			Reason:  "UnhealthyDependents",
			Message: "validation has not succeeded",
		},
	}
	if err := fakeClient.Status().Update(ctx, &overlay); err != nil {
		t.Fatalf("failed to set rejected overlay status: %v", err)
	}

	logs = nil
	if _, err := reconciler.Reconcile(ctx, req); err != nil {
		t.Fatalf("unexpected rejected-overlay reconcile error: %v", err)
	}

	readyGauge := m.OverlayReady.WithLabelValues(veneermetrics.CapacityTypePreference.String())
	countGauge := m.OverlayCount.WithLabelValues(veneermetrics.CapacityTypePreference.String())
	validationErrors := m.OverlayOperationErrorsTotal.WithLabelValues(
		veneermetrics.OperationUpdate.String(),
		veneermetrics.ErrorTypeValidation.String(),
	)
	if got := promtest.ToFloat64(countGauge); got != 1 {
		t.Fatalf("expected overlay count 1, got %v", got)
	}
	if got := promtest.ToFloat64(readyGauge); got != 0 {
		t.Fatalf("expected ready overlay count 0, got %v", got)
	}
	if got := promtest.ToFloat64(validationErrors); got != 1 {
		t.Fatalf("expected one validation error, got %v", got)
	}
	joinedLogs := strings.Join(logs, "\n")
	for _, want := range []string{
		"Karpenter rejected preference overlay",
		"pref-status-pool-1",
		"status-pool",
		"InvalidRequirement",
		"requirement key is not supported",
		`"errors":1`,
	} {
		if !strings.Contains(joinedLogs, want) {
			t.Errorf("expected logs to contain %q, got:\n%s", want, joinedLogs)
		}
	}

	if _, err := reconciler.Reconcile(ctx, req); err != nil {
		t.Fatalf("unexpected repeated rejected-overlay reconcile error: %v", err)
	}
	if got := promtest.ToFloat64(validationErrors); got != 1 {
		t.Fatalf("expected persistent failure not to increment counter again, got %v", got)
	}

	if err := fakeClient.Get(ctx, key, &overlay); err != nil {
		t.Fatalf("failed to get rejected overlay: %v", err)
	}
	overlay.Status.Conditions = []status.Condition{
		{Type: karpenterv1alpha1.ConditionTypeValidationSucceeded, Status: metav1.ConditionTrue},
		{Type: status.ConditionReady, Status: metav1.ConditionTrue},
	}
	if err := fakeClient.Status().Update(ctx, &overlay); err != nil {
		t.Fatalf("failed to set recovered overlay status: %v", err)
	}

	if _, err := reconciler.Reconcile(ctx, req); err != nil {
		t.Fatalf("unexpected recovered-overlay reconcile error: %v", err)
	}
	if got := promtest.ToFloat64(countGauge); got != 1 {
		t.Fatalf("expected overlay count to remain 1 after recovery, got %v", got)
	}
	if got := promtest.ToFloat64(readyGauge); got != 1 {
		t.Fatalf("expected ready overlay count 1 after recovery, got %v", got)
	}

	if err := fakeClient.Get(ctx, key, &overlay); err != nil {
		t.Fatalf("failed to get recovered overlay: %v", err)
	}
	overlay.Status.Conditions = []status.Condition{{
		Type:    karpenterv1alpha1.ConditionTypeValidationSucceeded,
		Status:  metav1.ConditionFalse,
		Reason:  "InvalidRequirement",
		Message: "requirement key is not supported",
	}}
	if err := fakeClient.Status().Update(ctx, &overlay); err != nil {
		t.Fatalf("failed to reject recovered overlay again: %v", err)
	}
	if _, err := reconciler.Reconcile(ctx, req); err != nil {
		t.Fatalf("unexpected second rejection reconcile error: %v", err)
	}
	if got := promtest.ToFloat64(validationErrors); got != 2 {
		t.Fatalf("expected recovery followed by rejection to increment counter to 2, got %v", got)
	}
}

// Helper functions
func strPtr(s string) *string {
	return &s
}

func int32Ptr(i int32) *int32 {
	return &i
}

// TestNodePoolReconciler_Reconcile_UpdatesPreferenceOverlayCount verifies that
// veneer_overlay_count{capacity_type="preference"} reflects the cluster-wide
// number of Veneer-managed preference overlays after a reconcile.
//
// The gauge has no NodePool dimension, so it must be recounted from a full
// list rather than derived from the overlays a single reconcile touched: a pass
// over pool-a must not clobber the contribution of pool-b.
func TestNodePoolReconciler_Reconcile_UpdatesPreferenceOverlayCount(t *testing.T) {
	scheme := setupTestScheme(t)

	poolA := &karpenterv1.NodePool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pool-a",
			Annotations: map[string]string{
				"veneer.io/preference.1": "karpenter.k8s.aws/instance-family=c7a adjust=-20%",
				"veneer.io/preference.2": "kubernetes.io/arch=arm64 adjust=+10%",
			},
		},
	}
	poolB := &karpenterv1.NodePool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pool-b",
			Annotations: map[string]string{
				"veneer.io/preference.1": "karpenter.k8s.aws/instance-family=m7g adjust=-15%",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(poolA, poolB).
		Build()

	reg := promclient.NewRegistry()
	m := veneermetrics.NewMetrics(reg)

	reconciler := &NodePoolReconciler{
		Client:    fakeClient,
		Logger:    logr.Discard(),
		Generator: preference.NewGenerator(),
		Metrics:   m,
	}

	gauge := m.OverlayCount.WithLabelValues(veneermetrics.CapacityTypePreference.String())
	readyGauge := m.OverlayReady.WithLabelValues(veneermetrics.CapacityTypePreference.String())

	// Seeded at registration, before anything has reconciled.
	if got := promtest.ToFloat64(gauge); got != 0 {
		t.Fatalf("expected seeded preference overlay count 0, got %v", got)
	}

	ctx := context.Background()

	result, err := reconciler.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "pool-a"},
	})
	if err != nil {
		t.Fatalf("unexpected error reconciling pool-a: %v", err)
	}
	if result.Requeue {
		t.Errorf("unexpected requeue")
	}
	if got := promtest.ToFloat64(gauge); got != 2 {
		t.Errorf("expected 2 preference overlays after reconciling pool-a, got %v", got)
	}
	if got := promtest.ToFloat64(readyGauge); got != 2 {
		t.Errorf("expected 2 ready preference overlays after reconciling pool-a, got %v", got)
	}

	// Reconciling a second pool must add to the total, not replace it.
	if _, err := reconciler.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "pool-b"},
	}); err != nil {
		t.Fatalf("unexpected error reconciling pool-b: %v", err)
	}
	if got := promtest.ToFloat64(gauge); got != 3 {
		t.Errorf("expected 3 preference overlays after reconciling pool-b, got %v", got)
	}
	if got := promtest.ToFloat64(readyGauge); got != 3 {
		t.Errorf("expected 3 ready preference overlays after reconciling pool-b, got %v", got)
	}

	// Deleting a NodePool cleans up its overlays and the count follows.
	if err := fakeClient.Delete(ctx, poolA); err != nil {
		t.Fatalf("failed to delete pool-a: %v", err)
	}
	if _, err := reconciler.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "pool-a"},
	}); err != nil {
		t.Fatalf("unexpected error reconciling deleted pool-a: %v", err)
	}
	if got := promtest.ToFloat64(gauge); got != 1 {
		t.Errorf("expected 1 preference overlay after pool-a cleanup, got %v", got)
	}
	if got := promtest.ToFloat64(readyGauge); got != 1 {
		t.Errorf("expected 1 ready preference overlay after pool-a cleanup, got %v", got)
	}
}

// TestNodePoolReconciler_Reconcile_NilMetricsIsSafe verifies the overlay-count
// bookkeeping is skipped rather than panicking when metrics are not wired up,
// which is how most of the other tests construct the reconciler.
func TestNodePoolReconciler_Reconcile_NilMetricsIsSafe(t *testing.T) {
	scheme := setupTestScheme(t)

	nodePool := &karpenterv1.NodePool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "no-metrics",
			Annotations: map[string]string{
				"veneer.io/preference.1": "karpenter.k8s.aws/instance-family=c7a adjust=-20%",
			},
		},
	}

	reconciler := &NodePoolReconciler{
		Client:    fake.NewClientBuilder().WithScheme(scheme).WithObjects(nodePool).Build(),
		Logger:    logr.Discard(),
		Generator: preference.NewGenerator(),
	}

	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "no-metrics"},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
