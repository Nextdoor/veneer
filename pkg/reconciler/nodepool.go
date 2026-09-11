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
	"fmt"
	"reflect"

	"github.com/awslabs/operatorpkg/status"
	"github.com/go-logr/logr"
	"github.com/nextdoor/veneer/pkg/metrics"
	"github.com/nextdoor/veneer/pkg/preference"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

	// observedFailures tracks the current failed condition for each overlay so
	// the validation counter records status transitions rather than increasing
	// on every reconcile while the same failure persists.
	observedFailures map[string]observedOverlayFailure
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
		if apierrors.IsNotFound(err) {
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

// updatePreferenceOverlayStatus refreshes the object and readiness gauges for
// preference overlays and reports objects that Karpenter rejected asynchronously.
//
// The gauges have no NodePool dimension but this reconciler is scoped to a
// single NodePool, so their values cannot be derived from the overlays one pass
// happened to touch -- reconciling pool-a must not clobber pool-b's
// contribution. They are therefore recounted from a full list served by the
// controller-runtime cache.
//
// A successful API write only proves that Kubernetes stored an overlay.
// Karpenter validates it later and reports runtime rejection through status
// conditions. Reading those conditions closes a silent-failure gap where an
// unusable overlay otherwise looked successful in every Veneer signal.
func (r *NodePoolReconciler) updatePreferenceOverlayStatus(
	ctx context.Context, log logr.Logger, reconciledNodePool string,
) (int, error) {
	var overlayList karpenterv1alpha1.NodeOverlayList
	if err := r.List(ctx, &overlayList, client.MatchingLabels{
		preference.LabelManagedBy:      preference.LabelManagedByValue,
		preference.LabelPreferenceType: preference.LabelPreferenceTypeValue,
	}); err != nil {
		log.Error(err, "Failed to inspect preference overlays for status")
		return 0, err
	}

	readyCount := 0
	unhealthyCount := 0
	seenFailures := make(map[string]struct{})
	for i := range overlayList.Items {
		overlay := &overlayList.Items[i]
		failedCondition := failedNodeOverlayCondition(overlay)
		if failedCondition == nil {
			readyCount++
			continue
		}

		sourceNodePool := preference.GetSourceNodePool(overlay)
		failure := observedOverlayFailureFromCondition(failedCondition)
		seenFailures[overlay.Name] = struct{}{}
		if sourceNodePool == reconciledNodePool {
			unhealthyCount++
			// Log every reconciliation while rejection persists so the current
			// operational failure remains visible. The counter below is separately
			// deduplicated and records only changes to the failed condition state.
			log.Error(
				fmt.Errorf("%s condition failed: %s", failedCondition.Type, failedCondition.Message),
				"Karpenter rejected preference overlay",
				"overlay", overlay.Name,
				"source_nodepool", sourceNodePool,
				"condition", failedCondition.Type,
				"reason", failedCondition.Reason,
				"message", failedCondition.Message,
			)
			if r.Metrics != nil && r.recordFailureTransition(overlay.Name, failure) {
				r.Metrics.RecordOverlayOperationError(metrics.OperationUpdate, metrics.ErrorTypeValidation)
			}
		}
	}

	r.clearRecoveredFailures(seenFailures)
	if r.Metrics != nil {
		r.Metrics.SetOverlayCount(metrics.CapacityTypePreference, len(overlayList.Items))
		r.Metrics.SetOverlayReady(metrics.CapacityTypePreference, readyCount)
	}
	return unhealthyCount, nil
}

// failedNodeOverlayCondition returns an explicit Karpenter failure condition
// that makes an overlay non-functional. ValidationSucceeded is preferred over
// the derived Ready condition so a single rejected overlay produces one error
// with the most specific reason. Missing or Unknown conditions are not failures
// because a newly created overlay may not have been validated yet.
func failedNodeOverlayCondition(overlay *karpenterv1alpha1.NodeOverlay) *status.Condition {
	conditionSet := overlay.StatusConditions(status.WithObservedOnly())
	validationCondition := conditionSet.Get(karpenterv1alpha1.ConditionTypeValidationSucceeded)
	if conditionIsCurrentFailure(overlay, validationCondition) {
		return validationCondition
	}
	if condition := conditionSet.Get(status.ConditionReady); conditionIsCurrentFailure(overlay, condition) {
		return condition
	}
	return nil
}

// conditionIsCurrentFailure ignores a failure from an older object generation.
// Karpenter may not have revalidated an updated spec yet, and reporting the old
// rejection during that gap would produce a false alarm.
func conditionIsCurrentFailure(overlay *karpenterv1alpha1.NodeOverlay, condition *status.Condition) bool {
	if !condition.IsFalse() {
		return false
	}
	return condition.ObservedGeneration == 0 || condition.ObservedGeneration == overlay.Generation
}

// observedOverlayFailure is the stable identity of one failed condition state.
// Reason and message changes are new observations even if the condition remains
// false, because they can identify a different Karpenter validation result.
type observedOverlayFailure struct {
	condition          string
	reason             string
	message            string
	observedGeneration int64
}

func observedOverlayFailureFromCondition(condition *status.Condition) observedOverlayFailure {
	return observedOverlayFailure{
		condition:          condition.Type,
		reason:             condition.Reason,
		message:            condition.Message,
		observedGeneration: condition.ObservedGeneration,
	}
}

func (r *NodePoolReconciler) recordFailureTransition(name string, failure observedOverlayFailure) bool {
	if r.observedFailures == nil {
		r.observedFailures = make(map[string]observedOverlayFailure)
	}
	if previous, found := r.observedFailures[name]; found && previous == failure {
		return false
	}
	r.observedFailures[name] = failure
	return true
}

func (r *NodePoolReconciler) clearRecoveredFailures(seen map[string]struct{}) {
	for name := range r.observedFailures {
		if _, failed := seen[name]; !failed {
			delete(r.observedFailures, name)
		}
	}
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
			// Update existing overlay only if its managed spec, labels, or owner differ.
			if !overlayNeedsUpdate(existingOverlay, desiredOverlay) {
				log.V(2).Info("Preference overlay already up to date", "overlay", name)
				continue
			}

			// Mutate a copy of the existing object so annotations, finalizers, and
			// labels owned by other tools survive Veneer's update. Legacy overlays
			// also gain a controller owner reference so the Owns watch receives their
			// status-only transitions.
			updatedOverlay := existingOverlay.DeepCopy()
			updatedOverlay.Spec = desiredOverlay.Spec
			updatedOverlay.OwnerReferences = desiredOverlay.OwnerReferences
			if updatedOverlay.Labels == nil {
				updatedOverlay.Labels = map[string]string{}
			}
			for key, value := range desiredOverlay.Labels {
				updatedOverlay.Labels[key] = value
			}
			if err := r.Update(ctx, updatedOverlay); err != nil {
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

	// Karpenter may reject an overlay after its API write succeeded. Read status
	// after CRUD so the summary and metrics reflect runtime validation failures,
	// not just Kubernetes API errors.
	unhealthyCount, err := r.updatePreferenceOverlayStatus(ctx, log, nodePool.Name)
	if err != nil {
		return ctrl.Result{}, err
	}
	errorCount += unhealthyCount

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
			Controller: boolPtr(true),
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
			if !apierrors.IsNotFound(err) {
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

	if _, err := r.updatePreferenceOverlayStatus(ctx, log, nodePoolName); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NodePoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&karpenterv1.NodePool{}).
		// Karpenter validates NodeOverlays asynchronously. Watching owned overlays
		// ensures a status-only rejection or recovery promptly re-reconciles the
		// source NodePool instead of remaining invisible until its next update.
		Owns(&karpenterv1alpha1.NodeOverlay{}).
		Complete(r)
}

func boolPtr(value bool) *bool {
	return &value
}

// overlayNeedsUpdate returns true if the existing overlay differs from the desired overlay
// in any meaningful way (spec or labels). This prevents unnecessary updates that would
// trigger additional reconciliation loops.
func overlayNeedsUpdate(existing, desired *karpenterv1alpha1.NodeOverlay) bool {
	// Ownership is part of the controller contract: existing overlays created
	// before the status watch must gain the controller bit so Owns can route their
	// status events back to this reconciler.
	if !reflect.DeepEqual(existing.Spec, desired.Spec) ||
		!reflect.DeepEqual(existing.OwnerReferences, desired.OwnerReferences) {
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
