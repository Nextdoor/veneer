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

// Helper functions
func strPtr(s string) *string {
	return &s
}

func int32Ptr(i int32) *int32 {
	return &i
}
