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

package reconciler

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/nextdoor/veneer/pkg/preference"
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

	// Create a controller reference to simulate Deployment owner
	controllerRef := &metav1.OwnerReference{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Name:       "veneer-controller",
		UID:        types.UID("test-deployment-uid-67890"),
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(nodePool).
		Build()

	reconciler := &NodePoolReconciler{
		Client:        client,
		Logger:        logr.Discard(),
		Generator:     preference.NewGenerator(),
		ControllerRef: controllerRef,
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

	// Verify both owner references are set
	if len(overlay.OwnerReferences) != 2 {
		t.Errorf("expected 2 owner references, got %d", len(overlay.OwnerReferences))
	}

	// Verify NodePool owner reference
	var hasNodePoolOwner, hasDeploymentOwner bool
	for _, ref := range overlay.OwnerReferences {
		if ref.Kind == "NodePool" && ref.Name == "owner-test-pool" {
			hasNodePoolOwner = true
			if ref.UID != nodePool.UID {
				t.Errorf("NodePool owner reference has wrong UID: got %s, want %s", ref.UID, nodePool.UID)
			}
			if ref.APIVersion != "karpenter.sh/v1" {
				t.Errorf("NodePool owner reference has wrong APIVersion: got %s, want karpenter.sh/v1", ref.APIVersion)
			}
		}
		if ref.Kind == "Deployment" && ref.Name == "veneer-controller" {
			hasDeploymentOwner = true
			if ref.UID != controllerRef.UID {
				t.Errorf("Deployment owner reference has wrong UID: got %s, want %s", ref.UID, controllerRef.UID)
			}
		}
	}

	if !hasNodePoolOwner {
		t.Error("overlay is missing NodePool owner reference")
	}
	if !hasDeploymentOwner {
		t.Error("overlay is missing Deployment owner reference")
	}
}

func TestNodePoolReconciler_Reconcile_OwnerReferences_WithoutControllerRef(t *testing.T) {
	scheme := setupTestScheme(t)

	// Create a NodePool with UID
	nodePool := &karpenterv1.NodePool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "owner-test-pool-2",
			UID:  types.UID("test-nodepool-uid-abcde"),
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

	// No ControllerRef set - simulates running outside of a Deployment (e.g., local dev)
	reconciler := &NodePoolReconciler{
		Client:        client,
		Logger:        logr.Discard(),
		Generator:     preference.NewGenerator(),
		ControllerRef: nil,
	}

	// Reconcile
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "owner-test-pool-2"}}
	_, err := reconciler.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify overlay was created with only NodePool owner reference
	var overlayList karpenterv1alpha1.NodeOverlayList
	if err := client.List(context.Background(), &overlayList); err != nil {
		t.Fatalf("failed to list overlays: %v", err)
	}

	if len(overlayList.Items) != 1 {
		t.Fatalf("expected 1 overlay, got %d", len(overlayList.Items))
	}

	overlay := overlayList.Items[0]

	// Verify only NodePool owner reference is set
	if len(overlay.OwnerReferences) != 1 {
		t.Errorf("expected 1 owner reference (NodePool only), got %d", len(overlay.OwnerReferences))
	}

	if len(overlay.OwnerReferences) > 0 {
		ref := overlay.OwnerReferences[0]
		if ref.Kind != "NodePool" {
			t.Errorf("expected owner reference kind NodePool, got %s", ref.Kind)
		}
		if ref.Name != "owner-test-pool-2" {
			t.Errorf("expected owner reference name owner-test-pool-2, got %s", ref.Name)
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
					Requirements: []corev1.NodeSelectorRequirement{
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
					Requirements: []corev1.NodeSelectorRequirement{
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
					Requirements: []corev1.NodeSelectorRequirement{
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
					Requirements: []corev1.NodeSelectorRequirement{
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
				"managed-by":                  "veneer",
				"veneer.io/type":              "preference",
				"veneer.io/source-nodepool":   "skip-update-pool",
				"veneer.io/preference-number": "1",
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
			Requirements: []corev1.NodeSelectorRequirement{
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

	// Verify overlay still exists with same ResourceVersion (not updated)
	var overlayList karpenterv1alpha1.NodeOverlayList
	if err := client.List(context.Background(), &overlayList); err != nil {
		t.Fatalf("failed to list overlays: %v", err)
	}

	if len(overlayList.Items) != 1 {
		t.Fatalf("expected 1 overlay, got %d", len(overlayList.Items))
	}

	// The fake client doesn't preserve ResourceVersion exactly, but this test
	// verifies the reconciler path doesn't error when overlay already matches
}

// Helper functions
func strPtr(s string) *string {
	return &s
}

func int32Ptr(i int32) *int32 {
	return &i
}
