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
	"reflect"

	"github.com/go-logr/logr"
	"github.com/nextdoor/veneer/pkg/metrics"
	"github.com/nextdoor/veneer/pkg/preference"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	karpenterv1alpha1 "sigs.k8s.io/karpenter/pkg/apis/v1alpha1"
)

// NodePoolReconciler watches NodePools and generates preference-based NodeOverlays
// from veneer.io/preference.N annotations.
//
// When a NodePool is created or updated, this reconciler:
// 1. Parses any veneer.io/preference.N annotations
// 2. Generates NodeOverlay resources for each valid preference
// 3. Creates new overlays, updates existing ones, and deletes stale ones
//
// When a NodePool is deleted, this reconciler cleans up all preference overlays
// that were generated from that NodePool.
type NodePoolReconciler struct {
	// Client is the Kubernetes client for managing resources
	client.Client

	// Logger is the structured logger for this reconciler
	Logger logr.Logger

	// Generator creates NodeOverlay specs from preferences
	Generator *preference.Generator

	// Metrics holds the Prometheus metrics for recording reconciler behavior
	Metrics *metrics.Metrics
}

// Reconcile handles NodePool create/update/delete events.
//
// For creates and updates:
//   - Parse preference annotations from the NodePool
//   - Generate desired NodeOverlay specs
//   - Compare with existing overlays and create/update/delete as needed
//
// For deletes:
//   - Find all preference overlays sourced from this NodePool
//   - Delete them
//
// +kubebuilder:rbac:groups=karpenter.sh,resources=nodepools,verbs=get;list;watch
// +kubebuilder:rbac:groups=karpenter.sh,resources=nodeoverlays,verbs=get;list;watch;create;update;delete
func (r *NodePoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Logger.WithValues("nodepool", req.Name)

	// Get the NodePool
	var nodePool karpenterv1.NodePool
	if err := r.Get(ctx, req.NamespacedName, &nodePool); err != nil {
		if errors.IsNotFound(err) {
			// NodePool was deleted - clean up any preference overlays from it
			log.Info("NodePool deleted, cleaning up preference overlays")
			return r.cleanupOverlaysForNodePool(ctx, req.Name)
		}
		log.Error(err, "Failed to get NodePool")
		return ctrl.Result{}, err
	}

	// Parse preference annotations
	prefs, parseErrors := preference.ParseNodePoolPreferences(nodePool.Annotations, nodePool.Name)
	for _, err := range parseErrors {
		log.Error(err, "Failed to parse preference annotation")
		if r.Metrics != nil {
			r.Metrics.RecordOverlayOperationError(metrics.OperationCreate, metrics.ErrorTypeValidation)
		}
	}

	// Generate desired overlays from preferences
	var desiredOverlays []*karpenterv1alpha1.NodeOverlay
	if r.Generator != nil && len(prefs) > 0 {
		desiredOverlays = r.Generator.GenerateAll(prefs)
	}

	// List existing preference overlays for this NodePool
	existingOverlays, err := r.listPreferenceOverlaysForNodePool(ctx, nodePool.Name)
	if err != nil {
		log.Error(err, "Failed to list existing preference overlays")
		return ctrl.Result{}, err
	}

	// Reconcile: create new, update existing, delete stale
	return r.reconcileOverlays(ctx, log, &nodePool, desiredOverlays, existingOverlays)
}

// listPreferenceOverlaysForNodePool returns all preference overlays generated from a NodePool.
func (r *NodePoolReconciler) listPreferenceOverlaysForNodePool(
	ctx context.Context, nodePoolName string,
) ([]karpenterv1alpha1.NodeOverlay, error) {
	var overlayList karpenterv1alpha1.NodeOverlayList
	if err := r.List(ctx, &overlayList, client.MatchingLabels{
		preference.LabelManagedBy:      preference.LabelManagedByValue,
		preference.LabelPreferenceType: preference.LabelPreferenceTypeValue,
		preference.LabelSourceNodePool: nodePoolName,
	}); err != nil {
		return nil, err
	}
	return overlayList.Items, nil
}

// reconcileOverlays compares desired vs existing overlays and performs CRUD operations.
func (r *NodePoolReconciler) reconcileOverlays(
	ctx context.Context,
	log logr.Logger,
	nodePool *karpenterv1.NodePool,
	desired []*karpenterv1alpha1.NodeOverlay,
	existing []karpenterv1alpha1.NodeOverlay,
) (ctrl.Result, error) {
	// Build map of existing overlays by name
	existingByName := make(map[string]*karpenterv1alpha1.NodeOverlay)
	for i := range existing {
		existingByName[existing[i].Name] = &existing[i]
	}

	// Build map of desired overlays by name
	desiredByName := make(map[string]*karpenterv1alpha1.NodeOverlay)
	for _, overlay := range desired {
		desiredByName[overlay.Name] = overlay
	}

	var createCount, updateCount, deleteCount, errorCount int

	// Create or update desired overlays
	for name, desiredOverlay := range desiredByName {
		existingOverlay, exists := existingByName[name]

		// Set owner references before create/update
		r.setOwnerReferences(desiredOverlay, nodePool)

		if !exists {
			// Create new overlay
			if err := r.Create(ctx, desiredOverlay); err != nil {
				log.Error(err, "Failed to create preference overlay", "overlay", name)
				if r.Metrics != nil {
					r.Metrics.RecordOverlayOperationError(metrics.OperationCreate, metrics.ErrorTypeAPI)
				}
				errorCount++
				continue
			}
			log.Info("Created preference overlay", "overlay", name)
			if r.Metrics != nil {
				r.Metrics.RecordOverlayOperation(metrics.OperationCreate, metrics.CapacityTypePreference)
			}
			createCount++
		} else {
			// Update existing overlay only if spec or labels actually differ
			if !overlayNeedsUpdate(existingOverlay, desiredOverlay) {
				log.V(2).Info("Preference overlay already up to date", "overlay", name)
				continue
			}

			// Copy resource version to allow update
			desiredOverlay.ResourceVersion = existingOverlay.ResourceVersion
			if err := r.Update(ctx, desiredOverlay); err != nil {
				log.Error(err, "Failed to update preference overlay", "overlay", name)
				if r.Metrics != nil {
					r.Metrics.RecordOverlayOperationError(metrics.OperationUpdate, metrics.ErrorTypeAPI)
				}
				errorCount++
				continue
			}
			log.V(1).Info("Updated preference overlay", "overlay", name)
			if r.Metrics != nil {
				r.Metrics.RecordOverlayOperation(metrics.OperationUpdate, metrics.CapacityTypePreference)
			}
			updateCount++
		}
	}

	// Delete stale overlays (exist but not desired)
	for name, existingOverlay := range existingByName {
		if _, desired := desiredByName[name]; !desired {
			if err := r.Delete(ctx, existingOverlay); err != nil {
				log.Error(err, "Failed to delete stale preference overlay", "overlay", name)
				if r.Metrics != nil {
					r.Metrics.RecordOverlayOperationError(metrics.OperationDelete, metrics.ErrorTypeAPI)
				}
				errorCount++
				continue
			}
			log.Info("Deleted stale preference overlay", "overlay", name)
			if r.Metrics != nil {
				r.Metrics.RecordOverlayOperation(metrics.OperationDelete, metrics.CapacityTypePreference)
			}
			deleteCount++
		}
	}

	if createCount > 0 || updateCount > 0 || deleteCount > 0 || errorCount > 0 {
		log.Info("Preference overlay reconciliation complete",
			"nodepool", nodePool.Name,
			"created", createCount,
			"updated", updateCount,
			"deleted", deleteCount,
			"errors", errorCount,
		)
	}

	return ctrl.Result{}, nil
}

// setOwnerReferences sets the owner references on a NodeOverlay.
// The overlay will be owned by the NodePool (source of the preference).
// This ensures overlays are garbage collected when the NodePool is deleted.
//
// Note: We only set NodePool as owner (not the controller Deployment) because
// Kubernetes OwnerReferences don't support cross-scope ownership. NodeOverlays
// are cluster-scoped, but Deployments are namespace-scoped. The OwnerReference
// struct has no namespace field, so the garbage collector cannot resolve
// namespace-scoped owners for cluster-scoped resources.
func (r *NodePoolReconciler) setOwnerReferences(
	overlay *karpenterv1alpha1.NodeOverlay, nodePool *karpenterv1.NodePool,
) {
	overlay.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: "karpenter.sh/v1",
			Kind:       "NodePool",
			Name:       nodePool.Name,
			UID:        nodePool.UID,
		},
	}
}

// cleanupOverlaysForNodePool deletes all preference overlays generated from a deleted NodePool.
func (r *NodePoolReconciler) cleanupOverlaysForNodePool(
	ctx context.Context, nodePoolName string,
) (ctrl.Result, error) {
	log := r.Logger.WithValues("nodepool", nodePoolName)

	// List all preference overlays for this NodePool
	overlays, err := r.listPreferenceOverlaysForNodePool(ctx, nodePoolName)
	if err != nil {
		log.Error(err, "Failed to list preference overlays for cleanup")
		return ctrl.Result{}, err
	}

	var deleteCount, errorCount int
	for i := range overlays {
		if err := r.Delete(ctx, &overlays[i]); err != nil {
			if !errors.IsNotFound(err) {
				log.Error(err, "Failed to delete preference overlay during cleanup", "overlay", overlays[i].Name)
				if r.Metrics != nil {
					r.Metrics.RecordOverlayOperationError(metrics.OperationDelete, metrics.ErrorTypeAPI)
				}
				errorCount++
			}
			continue
		}
		log.Info("Deleted preference overlay during cleanup", "overlay", overlays[i].Name)
		if r.Metrics != nil {
			r.Metrics.RecordOverlayOperation(metrics.OperationDelete, metrics.CapacityTypePreference)
		}
		deleteCount++
	}

	if deleteCount > 0 || errorCount > 0 {
		log.Info("Cleaned up preference overlays for deleted NodePool",
			"deleted", deleteCount,
			"errors", errorCount,
		)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NodePoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&karpenterv1.NodePool{}).
		Complete(r)
}

// overlayNeedsUpdate returns true if the existing overlay differs from the desired overlay
// in any meaningful way (spec or labels). This prevents unnecessary updates that would
// trigger additional reconciliation loops.
func overlayNeedsUpdate(existing, desired *karpenterv1alpha1.NodeOverlay) bool {
	// Compare specs using reflect.DeepEqual for the full spec comparison
	if !reflect.DeepEqual(existing.Spec, desired.Spec) {
		return true
	}

	// Compare labels - only the labels we manage
	// We need to check if all desired labels exist with correct values
	for key, desiredValue := range desired.Labels {
		if existingValue, ok := existing.Labels[key]; !ok || existingValue != desiredValue {
			return true
		}
	}

	return false
}
