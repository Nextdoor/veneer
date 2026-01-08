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

package overlay_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"

	"github.com/nextdoor/veneer/internal/testutil"
	"github.com/nextdoor/veneer/pkg/config"
	"github.com/nextdoor/veneer/pkg/overlay"
	"github.com/nextdoor/veneer/pkg/prometheus"
)

// TestDecisionEngineIntegration tests the full flow from Prometheus queries to overlay decisions.
// This integration test validates:
//  1. Prometheus client successfully queries metrics
//  2. Decision engine produces correct decisions based on real metric data
//  3. Multiple capacity types are handled correctly with appropriate precedence
//
//nolint:gocyclo // Integration test complexity is acceptable for comprehensive validation
func TestDecisionEngineIntegration(t *testing.T) {
	// Start mock Prometheus server with Lumina metrics
	fixture := testutil.LuminaMetricsWithSPUtilization()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle both GET and POST requests (Prometheus client uses POST)
		var query string
		if r.Method == http.MethodPost {
			if err := r.ParseForm(); err != nil {
				http.Error(w, "failed to parse form", http.StatusBadRequest)
				return
			}
			query = r.FormValue("query")
		} else {
			query = r.URL.Query().Get("query")
		}

		response, ok := fixture[query]
		if !ok {
			t.Logf("unexpected query: %s", query)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	defer mockServer.Close()

	// Create Prometheus client pointing to mock server
	promClient, err := prometheus.NewClient(mockServer.URL, "123456789012", "us-west-2", logr.Discard())
	if err != nil {
		t.Fatalf("failed to create Prometheus client: %v", err)
	}

	// Create decision engine with test config
	cfg := &config.Config{
		Overlays: config.OverlayManagementConfig{
			UtilizationThreshold: 95.0,
			Weights: config.OverlayWeightsConfig{
				ReservedInstance:       30,
				EC2InstanceSavingsPlan: 20,
				ComputeSavingsPlan:     10,
			},
		},
	}
	engine := overlay.NewDecisionEngine(cfg)

	ctx := context.Background()

	// Test 1: Query and analyze Compute Savings Plan
	t.Run("compute savings plan decision", func(t *testing.T) {
		// Query utilization
		utilizations, err := promClient.QuerySavingsPlanUtilization(ctx, prometheus.SavingsPlanTypeCompute)
		if err != nil {
			t.Fatalf("failed to query SP utilization: %v", err)
		}
		if len(utilizations) == 0 {
			t.Fatal("expected at least one Compute SP utilization metric")
		}

		// Query capacity
		capacities, err := promClient.QuerySavingsPlanCapacity(ctx, "")
		if err != nil {
			t.Fatalf("failed to query SP capacity: %v", err)
		}

		// Find Compute SP capacity (match by ARN)
		var computeCapacity prometheus.SavingsPlanCapacity
		for _, cap := range capacities {
			if cap.Type == prometheus.SavingsPlanTypeCompute && cap.SavingsPlanARN == utilizations[0].SavingsPlanARN {
				computeCapacity = cap
				break
			}
		}
		if computeCapacity.SavingsPlanARN == "" {
			t.Fatal("failed to find matching Compute SP capacity")
		}

		// Analyze and make decision
		decision := engine.AnalyzeComputeSavingsPlanSingle(utilizations[0], computeCapacity)

		// Validate decision
		if decision.Name != "cost-aware-compute-sp-global" {
			t.Errorf("Name = %q, want %q", decision.Name, "cost-aware-compute-sp-global")
		}

		if decision.CapacityType != overlay.CapacityTypeComputeSavingsPlan {
			t.Errorf("CapacityType = %q, want %q", decision.CapacityType, overlay.CapacityTypeComputeSavingsPlan)
		}

		if decision.Weight != 10 {
			t.Errorf("Weight = %d, want 10", decision.Weight)
		}

		// Fixture has 87.5% utilization (below 95% threshold)
		if !decision.ShouldExist {
			t.Errorf("ShouldExist = false, want true (utilization %.1f%% is below threshold %.1f%%)",
				utilizations[0].UtilizationPercent, cfg.Overlays.UtilizationThreshold)
		}

		if decision.UtilizationPercent != 87.5 {
			t.Errorf("UtilizationPercent = %f, want 87.5", decision.UtilizationPercent)
		}

		// Verify selector targets all families with on-demand capacity
		expectedSelector := "karpenter.k8s.aws/instance-family: Exists, karpenter.sh/capacity-type: In [on-demand]"
		if decision.TargetSelector != expectedSelector {
			t.Errorf("TargetSelector = %q, want %q", decision.TargetSelector, expectedSelector)
		}
	})

	// Test 2: Query and analyze EC2 Instance Savings Plan
	t.Run("ec2 instance savings plan decision", func(t *testing.T) {
		// Query utilization for EC2 Instance SPs
		utilizations, err := promClient.QuerySavingsPlanUtilization(ctx, prometheus.SavingsPlanTypeEC2Instance)
		if err != nil {
			t.Fatalf("failed to query EC2 Instance SP utilization: %v", err)
		}
		if len(utilizations) == 0 {
			t.Fatal("expected at least one EC2 Instance SP utilization metric")
		}

		// Query capacity for m5 family
		capacities, err := promClient.QuerySavingsPlanCapacity(ctx, "m5")
		if err != nil {
			t.Fatalf("failed to query EC2 Instance SP capacity: %v", err)
		}
		if len(capacities) == 0 {
			t.Fatal("expected at least one EC2 Instance SP capacity metric for m5 family")
		}

		// Find matching capacity
		var m5Capacity prometheus.SavingsPlanCapacity
		for _, cap := range capacities {
			if cap.Type == prometheus.SavingsPlanTypeEC2Instance && cap.InstanceFamily == "m5" {
				m5Capacity = cap
				break
			}
		}

		// Analyze and make decision
		decision := engine.AnalyzeEC2InstanceSavingsPlanSingle(utilizations[0], m5Capacity)

		// Validate decision
		if decision.Name != "cost-aware-ec2-sp-m5-us-west-2" {
			t.Errorf("Name = %q, want %q", decision.Name, "cost-aware-ec2-sp-m5-us-west-2")
		}

		if decision.CapacityType != overlay.CapacityTypeEC2InstanceSavingsPlan {
			t.Errorf("CapacityType = %q, want %q", decision.CapacityType, overlay.CapacityTypeEC2InstanceSavingsPlan)
		}

		if decision.Weight != 20 {
			t.Errorf("Weight = %d, want 20", decision.Weight)
		}

		// Fixture has 96.2% utilization (above 95% threshold)
		if decision.ShouldExist {
			t.Errorf("ShouldExist = true, want false (utilization %.1f%% is above threshold %.1f%%)",
				utilizations[0].UtilizationPercent, cfg.Overlays.UtilizationThreshold)
		}

		// Use epsilon comparison for floating point
		const epsilon = 0.01
		if diff := decision.UtilizationPercent - 96.2; diff < -epsilon || diff > epsilon {
			t.Errorf("UtilizationPercent = %f, want 96.2 (diff: %e)", decision.UtilizationPercent, diff)
		}

		// Verify selector targets m5 family with on-demand capacity
		expectedSelector := "karpenter.k8s.aws/instance-family: In [m5], karpenter.sh/capacity-type: In [on-demand]"
		if decision.TargetSelector != expectedSelector {
			t.Errorf("TargetSelector = %q, want %q", decision.TargetSelector, expectedSelector)
		}
	})

	// Test 3: Query and analyze Reserved Instances
	t.Run("reserved instance decision", func(t *testing.T) {
		// Query RIs for m5.xlarge
		ris, err := promClient.QueryReservedInstances(ctx, "m5.xlarge")
		if err != nil {
			t.Fatalf("failed to query reserved instances: %v", err)
		}
		if len(ris) == 0 {
			t.Fatal("expected at least one RI metric for m5.xlarge")
		}

		// Analyze and make decision
		decision := engine.AnalyzeReservedInstanceSingle(ris[0])

		// Validate decision
		if decision.Name != "cost-aware-ri-m5.xlarge-us-west-2" {
			t.Errorf("Name = %q, want %q", decision.Name, "cost-aware-ri-m5.xlarge-us-west-2")
		}

		if decision.CapacityType != overlay.CapacityTypeReservedInstance {
			t.Errorf("CapacityType = %q, want %q", decision.CapacityType, overlay.CapacityTypeReservedInstance)
		}

		if decision.Weight != 30 {
			t.Errorf("Weight = %d, want 30", decision.Weight)
		}

		// Fixture has 2 RIs available (should exist)
		if !decision.ShouldExist {
			t.Errorf("ShouldExist = false, want true (count = %d)", ris[0].Count)
		}

		// Verify selector targets specific instance type with on-demand capacity
		expectedSelector := "node.kubernetes.io/instance-type: In [m5.xlarge], karpenter.sh/capacity-type: In [on-demand]"
		if decision.TargetSelector != expectedSelector {
			t.Errorf("TargetSelector = %q, want %q", decision.TargetSelector, expectedSelector)
		}
	})
}

// TestMultipleCapacityTypesIntegration tests handling of multiple capacity types simultaneously.
// This validates the full workflow when multiple overlays need to be managed concurrently.
//
//nolint:gocyclo // Integration test complexity is acceptable for comprehensive validation
func TestMultipleCapacityTypesIntegration(t *testing.T) {
	// Start mock Prometheus server
	fixture := testutil.LuminaMetricsWithSPUtilization()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle both GET and POST requests (Prometheus client uses POST)
		var query string
		if r.Method == http.MethodPost {
			if err := r.ParseForm(); err != nil {
				http.Error(w, "failed to parse form", http.StatusBadRequest)
				return
			}
			query = r.FormValue("query")
		} else {
			query = r.URL.Query().Get("query")
		}

		response, ok := fixture[query]
		if !ok {
			t.Logf("unexpected query: %s", query)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	defer mockServer.Close()

	promClient, err := prometheus.NewClient(mockServer.URL, "123456789012", "us-west-2", logr.Discard())
	if err != nil {
		t.Fatalf("failed to create Prometheus client: %v", err)
	}

	cfg := &config.Config{
		Overlays: config.OverlayManagementConfig{
			UtilizationThreshold: 95.0,
			Weights: config.OverlayWeightsConfig{
				ReservedInstance:       30,
				EC2InstanceSavingsPlan: 20,
				ComputeSavingsPlan:     10,
			},
		},
	}
	engine := overlay.NewDecisionEngine(cfg)

	ctx := context.Background()

	// Query all capacity types
	decisions := make([]overlay.Decision, 0, 10) // Pre-allocate for expected capacity types

	// 1. Compute SPs
	computeUtils, err := promClient.QuerySavingsPlanUtilization(ctx, prometheus.SavingsPlanTypeCompute)
	if err != nil {
		t.Fatalf("failed to query Compute SP utilization: %v", err)
	}
	computeCaps, err := promClient.QuerySavingsPlanCapacity(ctx, "")
	if err != nil {
		t.Fatalf("failed to query SP capacity: %v", err)
	}

	for _, util := range computeUtils {
		// Find matching capacity
		for _, cap := range computeCaps {
			if cap.Type == prometheus.SavingsPlanTypeCompute && cap.SavingsPlanARN == util.SavingsPlanARN {
				decision := engine.AnalyzeComputeSavingsPlanSingle(util, cap)
				decisions = append(decisions, decision)
				break
			}
		}
	}

	// 2. EC2 Instance SPs
	ec2Utils, err := promClient.QuerySavingsPlanUtilization(ctx, prometheus.SavingsPlanTypeEC2Instance)
	if err != nil {
		t.Fatalf("failed to query EC2 Instance SP utilization: %v", err)
	}

	for _, util := range ec2Utils {
		// Find matching capacity
		familyCaps, err := promClient.QuerySavingsPlanCapacity(ctx, util.InstanceFamily)
		if err != nil {
			t.Fatalf("failed to query SP capacity for family %s: %v", util.InstanceFamily, err)
		}
		for _, cap := range familyCaps {
			if cap.Type == prometheus.SavingsPlanTypeEC2Instance && cap.InstanceFamily == util.InstanceFamily {
				decision := engine.AnalyzeEC2InstanceSavingsPlanSingle(util, cap)
				decisions = append(decisions, decision)
				break
			}
		}
	}

	// 3. Reserved Instances
	ris, err := promClient.QueryReservedInstances(ctx, "")
	if err != nil {
		t.Fatalf("failed to query reserved instances: %v", err)
	}
	for _, ri := range ris {
		decision := engine.AnalyzeReservedInstanceSingle(ri)
		decisions = append(decisions, decision)
	}

	// Validate we got decisions for all capacity types
	if len(decisions) == 0 {
		t.Fatal("expected at least one decision")
	}

	// Verify weight ordering (highest to lowest priority)
	// RIs should have weight 30, EC2 Instance SPs weight 20, Compute SPs weight 10
	hasRI := false
	hasEC2SP := false
	hasComputeSP := false

	for _, decision := range decisions {
		switch decision.CapacityType {
		case overlay.CapacityTypeReservedInstance:
			hasRI = true
			if decision.Weight != 30 {
				t.Errorf("RI weight = %d, want 30", decision.Weight)
			}
		case overlay.CapacityTypeEC2InstanceSavingsPlan:
			hasEC2SP = true
			if decision.Weight != 20 {
				t.Errorf("EC2 Instance SP weight = %d, want 20", decision.Weight)
			}
		case overlay.CapacityTypeComputeSavingsPlan:
			hasComputeSP = true
			if decision.Weight != 10 {
				t.Errorf("Compute SP weight = %d, want 10", decision.Weight)
			}
		}

		// Verify all decisions have required fields
		if decision.Name == "" {
			t.Errorf("decision has empty Name")
		}
		if decision.Price != "0.00" {
			t.Errorf("decision Price = %q, want \"0.00\"", decision.Price)
		}
		if decision.TargetSelector == "" {
			t.Errorf("decision has empty TargetSelector")
		}
		if decision.Reason == "" {
			t.Errorf("decision has empty Reason")
		}
	}

	// Verify we found all capacity types in test fixture
	if !hasRI {
		t.Error("no RI decisions found")
	}
	if !hasEC2SP {
		t.Error("no EC2 Instance SP decisions found")
	}
	if !hasComputeSP {
		t.Error("no Compute SP decisions found")
	}

	t.Logf("Successfully analyzed %d capacity sources:", len(decisions))
	for _, decision := range decisions {
		t.Logf("  - %s (type=%s, weight=%d, should_exist=%v, reason=%s)",
			decision.Name, decision.CapacityType, decision.Weight, decision.ShouldExist, decision.Reason)
	}
}

// TestNodeOverlayGeneratorIntegration tests the full flow from Prometheus queries to NodeOverlay generation.
// This validates Phase 4 functionality: generating valid NodeOverlay specs from capacity decisions.
//
//nolint:gocyclo // Integration test complexity is acceptable for comprehensive validation
func TestNodeOverlayGeneratorIntegration(t *testing.T) {
	// Start mock Prometheus server with Lumina metrics
	fixture := testutil.LuminaMetricsWithSPUtilization()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var query string
		if r.Method == http.MethodPost {
			if err := r.ParseForm(); err != nil {
				http.Error(w, "failed to parse form", http.StatusBadRequest)
				return
			}
			query = r.FormValue("query")
		} else {
			query = r.URL.Query().Get("query")
		}

		response, ok := fixture[query]
		if !ok {
			t.Logf("unexpected query: %s", query)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	defer mockServer.Close()

	// Create Prometheus client pointing to mock server
	promClient, err := prometheus.NewClient(mockServer.URL, "123456789012", "us-west-2", logr.Discard())
	if err != nil {
		t.Fatalf("failed to create Prometheus client: %v", err)
	}

	// Create decision engine with test config
	cfg := &config.Config{
		Overlays: config.OverlayManagementConfig{
			UtilizationThreshold: 95.0,
			Weights: config.OverlayWeightsConfig{
				ReservedInstance:       30,
				EC2InstanceSavingsPlan: 20,
				ComputeSavingsPlan:     10,
			},
		},
	}
	engine := overlay.NewDecisionEngine(cfg)
	generator := overlay.NewGenerator()

	ctx := context.Background()

	// Collect decisions from all capacity types
	var decisions []overlay.Decision

	// Query Compute SPs
	computeUtils, err := promClient.QuerySavingsPlanUtilization(ctx, prometheus.SavingsPlanTypeCompute)
	if err != nil {
		t.Fatalf("failed to query Compute SP utilization: %v", err)
	}
	computeCaps, err := promClient.QuerySavingsPlanCapacity(ctx, "")
	if err != nil {
		t.Fatalf("failed to query SP capacity: %v", err)
	}
	for _, util := range computeUtils {
		for _, cap := range computeCaps {
			if cap.Type == prometheus.SavingsPlanTypeCompute && cap.SavingsPlanARN == util.SavingsPlanARN {
				decisions = append(decisions, engine.AnalyzeComputeSavingsPlanSingle(util, cap))
				break
			}
		}
	}

	// Query EC2 Instance SPs
	ec2Utils, err := promClient.QuerySavingsPlanUtilization(ctx, prometheus.SavingsPlanTypeEC2Instance)
	if err != nil {
		t.Fatalf("failed to query EC2 Instance SP utilization: %v", err)
	}
	for _, util := range ec2Utils {
		familyCaps, err := promClient.QuerySavingsPlanCapacity(ctx, util.InstanceFamily)
		if err != nil {
			t.Fatalf("failed to query SP capacity for family %s: %v", util.InstanceFamily, err)
		}
		for _, cap := range familyCaps {
			if cap.Type == prometheus.SavingsPlanTypeEC2Instance && cap.InstanceFamily == util.InstanceFamily {
				decisions = append(decisions, engine.AnalyzeEC2InstanceSavingsPlanSingle(util, cap))
				break
			}
		}
	}

	// Query Reserved Instances
	ris, err := promClient.QueryReservedInstances(ctx, "")
	if err != nil {
		t.Fatalf("failed to query reserved instances: %v", err)
	}
	for _, ri := range ris {
		decisions = append(decisions, engine.AnalyzeReservedInstanceSingle(ri))
	}

	// Generate NodeOverlays from decisions
	generatedOverlays := generator.GenerateAll(decisions)

	t.Run("generates overlays for decisions that should exist", func(t *testing.T) {
		createCount := 0
		deleteCount := 0

		for _, gen := range generatedOverlays {
			if gen.Action == "create" {
				createCount++
				if gen.Overlay == nil {
					t.Error("overlay with action=create has nil Overlay")
				}
			} else if gen.Action == "delete" {
				deleteCount++
				if gen.Overlay != nil {
					t.Error("overlay with action=delete should have nil Overlay")
				}
			}
		}

		t.Logf("Generated %d overlays to create, %d to delete", createCount, deleteCount)

		if createCount == 0 && deleteCount == 0 {
			t.Error("expected at least one overlay action")
		}
	})

	t.Run("generated overlays have valid metadata", func(t *testing.T) {
		for _, gen := range generatedOverlays {
			if gen.Overlay == nil {
				continue // Skip delete actions
			}

			// Verify name matches decision
			if gen.Overlay.Name != gen.Decision.Name {
				t.Errorf("overlay name %q doesn't match decision name %q",
					gen.Overlay.Name, gen.Decision.Name)
			}

			// Verify managed-by label
			if gen.Overlay.Labels[overlay.LabelManagedBy] != overlay.LabelManagedByValue {
				t.Errorf("overlay %s missing managed-by label", gen.Overlay.Name)
			}

			// Verify capacity-type label
			if gen.Overlay.Labels[overlay.LabelCapacityType] == "" {
				t.Errorf("overlay %s missing capacity-type label", gen.Overlay.Name)
			}

			// Verify optimization-reason label
			if gen.Overlay.Labels[overlay.LabelOptimizationReason] == "" {
				t.Errorf("overlay %s missing optimization-reason label", gen.Overlay.Name)
			}
		}
	})

	t.Run("generated overlays have valid specs", func(t *testing.T) {
		for _, gen := range generatedOverlays {
			if gen.Overlay == nil {
				continue
			}

			// Verify price
			if gen.Overlay.Spec.Price == nil || *gen.Overlay.Spec.Price != "0.00" {
				t.Errorf("overlay %s has invalid price: %v", gen.Overlay.Name, gen.Overlay.Spec.Price)
			}

			// Verify weight matches decision
			if gen.Overlay.Spec.Weight == nil {
				t.Errorf("overlay %s has nil weight", gen.Overlay.Name)
			} else if int(*gen.Overlay.Spec.Weight) != gen.Decision.Weight {
				t.Errorf("overlay %s weight %d doesn't match decision weight %d",
					gen.Overlay.Name, *gen.Overlay.Spec.Weight, gen.Decision.Weight)
			}

			// Verify requirements exist
			if len(gen.Overlay.Spec.Requirements) == 0 {
				t.Errorf("overlay %s has no requirements", gen.Overlay.Name)
			}

			// Verify all overlays target on-demand capacity
			hasOnDemandReq := false
			for _, req := range gen.Overlay.Spec.Requirements {
				if req.Key == overlay.LabelCapacityTypeKarpenter &&
					req.Operator == corev1.NodeSelectorOpIn &&
					len(req.Values) == 1 && req.Values[0] == "on-demand" {
					hasOnDemandReq = true
					break
				}
			}
			if !hasOnDemandReq {
				t.Errorf("overlay %s doesn't target on-demand capacity", gen.Overlay.Name)
			}
		}
	})

	t.Run("generated overlays pass validation", func(t *testing.T) {
		for _, gen := range generatedOverlays {
			if gen.Overlay == nil {
				continue
			}

			errors := overlay.ValidateOverlay(gen.Overlay)
			if len(errors) > 0 {
				t.Errorf("overlay %s failed validation: %v", gen.Overlay.Name, errors)
			}
		}
	})

	t.Run("overlay requirements match capacity type", func(t *testing.T) {
		for _, gen := range generatedOverlays {
			if gen.Overlay == nil {
				continue
			}

			switch gen.Decision.CapacityType {
			case overlay.CapacityTypeComputeSavingsPlan:
				// Should have instance-family Exists requirement
				hasExistsReq := false
				for _, req := range gen.Overlay.Spec.Requirements {
					if req.Key == overlay.LabelInstanceFamilyKarpenter &&
						req.Operator == corev1.NodeSelectorOpExists {
						hasExistsReq = true
						break
					}
				}
				if !hasExistsReq {
					t.Errorf("Compute SP overlay %s should have instance-family Exists requirement",
						gen.Overlay.Name)
				}

			case overlay.CapacityTypeEC2InstanceSavingsPlan:
				// Should have instance-family In requirement
				hasInReq := false
				for _, req := range gen.Overlay.Spec.Requirements {
					if req.Key == overlay.LabelInstanceFamilyKarpenter &&
						req.Operator == corev1.NodeSelectorOpIn &&
						len(req.Values) > 0 {
						hasInReq = true
						break
					}
				}
				if !hasInReq {
					t.Errorf("EC2 Instance SP overlay %s should have instance-family In requirement",
						gen.Overlay.Name)
				}

			case overlay.CapacityTypeReservedInstance:
				// Should have instance-type In requirement
				hasTypeReq := false
				for _, req := range gen.Overlay.Spec.Requirements {
					if req.Key == overlay.LabelInstanceTypeK8s &&
						req.Operator == corev1.NodeSelectorOpIn &&
						len(req.Values) > 0 {
						hasTypeReq = true
						break
					}
				}
				if !hasTypeReq {
					t.Errorf("RI overlay %s should have instance-type In requirement",
						gen.Overlay.Name)
				}
			}
		}
	})

	t.Run("YAML output is well-formed", func(t *testing.T) {
		for _, gen := range generatedOverlays {
			if gen.Overlay == nil {
				continue
			}

			yaml := overlay.FormatOverlayYAML(gen.Overlay)

			// Verify YAML contains expected fields
			if !strings.Contains(yaml, "apiVersion: karpenter.sh/v1alpha1") {
				t.Errorf("YAML for %s missing apiVersion", gen.Overlay.Name)
			}
			if !strings.Contains(yaml, "kind: NodeOverlay") {
				t.Errorf("YAML for %s missing kind", gen.Overlay.Name)
			}
			if !strings.Contains(yaml, "name: "+gen.Overlay.Name) {
				t.Errorf("YAML for %s missing name", gen.Overlay.Name)
			}
			if !strings.Contains(yaml, `price: "0.00"`) {
				t.Errorf("YAML for %s missing price", gen.Overlay.Name)
			}
			if !strings.Contains(yaml, "requirements:") {
				t.Errorf("YAML for %s missing requirements", gen.Overlay.Name)
			}

			t.Logf("Generated YAML for %s:\n%s", gen.Overlay.Name, yaml)
		}
	})
}
