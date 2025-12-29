/*
Copyright 2025 Karve Contributors.

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
	"testing"

	"github.com/go-logr/logr"
	"github.com/nextdoor/karve/internal/testutil"
	"github.com/nextdoor/karve/pkg/config"
	"github.com/nextdoor/karve/pkg/overlay"
	"github.com/nextdoor/karve/pkg/prometheus"
)

// TestMultipleSavingsPlansAggregation verifies that multiple Savings Plans of the same type
// are properly aggregated to prevent duplicate overlay names.
//
// Scenario:
// - 3 Compute SPs (all global, targeting ALL instance families)
// - 2 EC2 Instance SPs for m5 family (both targeting m5 instances)
// - 2 RIs for m5.xlarge (both targeting m5.xlarge instances)
//
// Expected Behavior:
// - Compute SPs should be aggregated into ONE decision: "cost-aware-compute-sp-global"
// - EC2 Instance SPs (m5) should be aggregated into ONE decision: "cost-aware-ec2-sp-m5"
// - RIs (m5.xlarge) should be aggregated into ONE decision: "cost-aware-ri-m5.xlarge"
func TestMultipleSavingsPlansAggregation(t *testing.T) {

	// Start mock Prometheus server with multiple overlapping SPs
	fixture := testutil.LuminaMetricsWithMultipleSPs()
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

	// Query all Compute SPs (expecting 3)
	computeUtils, err := promClient.QuerySavingsPlanUtilization(ctx, prometheus.SavingsPlanTypeCompute)
	if err != nil {
		t.Fatalf("failed to query Compute SP utilization: %v", err)
	}
	if len(computeUtils) != 3 {
		t.Fatalf("expected 3 Compute SPs, got %d", len(computeUtils))
	}

	computeCaps, err := promClient.QuerySavingsPlanCapacity(ctx, "")
	if err != nil {
		t.Fatalf("failed to query SP capacity: %v", err)
	}

	// Filter to get only Compute SP capacities
	var computeCapacities []prometheus.SavingsPlanCapacity
	for _, cap := range computeCaps {
		if cap.Type == prometheus.SavingsPlanTypeCompute {
			computeCapacities = append(computeCapacities, cap)
		}
	}

	// Aggregate the 3 Compute SPs into one
	agg := overlay.AggregateComputeSavingsPlans(computeUtils, computeCapacities)

	// Verify aggregation
	if agg.Count != 3 {
		t.Errorf("expected Count=3, got %d", agg.Count)
	}

	// Expected total capacity: 5 + 10 + 15 = 30.00 $/hour
	expectedCapacity := 30.0
	if agg.TotalRemainingCapacity != expectedCapacity {
		t.Errorf("expected TotalRemainingCapacity=%.2f, got %.2f", expectedCapacity, agg.TotalRemainingCapacity)
	}

	// Create ONE decision from aggregated data
	decision := engine.AnalyzeComputeSavingsPlan(agg)

	// Verify single decision with correct name
	if decision.Name != "cost-aware-compute-sp-global" {
		t.Errorf("expected Name='cost-aware-compute-sp-global', got '%s'", decision.Name)
	}

	// Verify capacity is aggregated
	if decision.RemainingCapacity != expectedCapacity {
		t.Errorf("expected RemainingCapacity=%.2f, got %.2f", expectedCapacity, decision.RemainingCapacity)
	}

	// Verify overlay should exist (utilization below threshold, capacity available)
	if !decision.ShouldExist {
		t.Errorf("expected ShouldExist=true, got false. Reason: %s", decision.Reason)
	}

	t.Logf("SUCCESS: Created 1 aggregated decision with capacity=%.2f $/hour", decision.RemainingCapacity)
}

// TestMultipleEC2InstanceSPsAggregation verifies that EC2 Instance SPs for the same
// instance family are properly aggregated to prevent duplicate overlay names.
func TestMultipleEC2InstanceSPsAggregation(t *testing.T) {

	fixture := testutil.LuminaMetricsWithMultipleSPs()
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

	// Query all EC2 Instance SPs (expecting 3: 2 for m5, 1 for c5)
	ec2Utils, err := promClient.QuerySavingsPlanUtilization(ctx, prometheus.SavingsPlanTypeEC2Instance)
	if err != nil {
		t.Fatalf("failed to query EC2 Instance SP utilization: %v", err)
	}
	if len(ec2Utils) != 3 {
		t.Fatalf("expected 3 EC2 Instance SPs, got %d", len(ec2Utils))
	}

	ec2Caps, err := promClient.QuerySavingsPlanCapacity(ctx, "")
	if err != nil {
		t.Fatalf("failed to query SP capacity: %v", err)
	}

	// Filter to get only EC2 Instance SP capacities
	var ec2Capacities []prometheus.SavingsPlanCapacity
	for _, cap := range ec2Caps {
		if cap.Type == prometheus.SavingsPlanTypeEC2Instance {
			ec2Capacities = append(ec2Capacities, cap)
		}
	}

	// Aggregate EC2 Instance SPs by family
	aggByFamily := overlay.AggregateEC2InstanceSavingsPlans(ec2Utils, ec2Capacities)

	// Verify we have 2 families: m5 and c5
	if len(aggByFamily) != 2 {
		t.Fatalf("expected 2 families, got %d", len(aggByFamily))
	}

	// Test m5 aggregation (2 SPs) - now keyed by "family:region"
	m5Key := "m5:us-west-2"
	m5Agg, ok := aggByFamily[m5Key]
	if !ok {
		keys := make([]string, 0, len(aggByFamily))
		for k := range aggByFamily {
			keys = append(keys, k)
		}
		t.Fatalf("expected %s in aggregation, got keys: %v", m5Key, keys)
	}

	if m5Agg.Count != 2 {
		t.Errorf("expected m5 Count=2, got %d", m5Agg.Count)
	}

	// Expected m5 capacity: 8 + 12 = 20.00 $/hour
	expectedM5Capacity := 20.0
	if m5Agg.TotalRemainingCapacity != expectedM5Capacity {
		t.Errorf("expected m5 TotalRemainingCapacity=%.2f, got %.2f", expectedM5Capacity, m5Agg.TotalRemainingCapacity)
	}

	// Create decision from aggregated m5 data
	m5Decision := engine.AnalyzeEC2InstanceSavingsPlan(m5Agg)

	if m5Decision.Name != "cost-aware-ec2-sp-m5-us-west-2" {
		t.Errorf("expected Name='cost-aware-ec2-sp-m5-us-west-2', got '%s'", m5Decision.Name)
	}

	if m5Decision.RemainingCapacity != expectedM5Capacity {
		t.Errorf("expected RemainingCapacity=%.2f, got %.2f", expectedM5Capacity, m5Decision.RemainingCapacity)
	}

	if !m5Decision.ShouldExist {
		t.Errorf("expected ShouldExist=true, got false. Reason: %s", m5Decision.Reason)
	}

	// Test c5 aggregation (1 SP) - now keyed by "family:region"
	c5Key := "c5:us-west-2"
	c5Agg, ok := aggByFamily[c5Key]
	if !ok {
		keys := make([]string, 0, len(aggByFamily))
		for k := range aggByFamily {
			keys = append(keys, k)
		}
		t.Fatalf("expected %s in aggregation, got keys: %v", c5Key, keys)
	}

	if c5Agg.Count != 1 {
		t.Errorf("expected c5 Count=1, got %d", c5Agg.Count)
	}

	expectedC5Capacity := 7.0 // Based on LuminaMetricsWithMultipleSPs fixture
	if c5Agg.TotalRemainingCapacity != expectedC5Capacity {
		t.Errorf("expected c5 TotalRemainingCapacity=%.2f, got %.2f", expectedC5Capacity, c5Agg.TotalRemainingCapacity)
	}

	t.Logf("SUCCESS: Created 2 aggregated decisions (m5 with %.2f $/hour, c5 with %.2f $/hour)",
		m5Decision.RemainingCapacity, c5Agg.TotalRemainingCapacity)
}

// TestMultipleReservedInstancesAggregation verifies that Reserved Instances for the same
// instance type across different AZs are properly aggregated to prevent duplicate overlay names.
func TestMultipleReservedInstancesAggregation(t *testing.T) {

	fixture := testutil.LuminaMetricsWithMultipleSPs()
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

	// Query all RIs (expecting 2 entries for m5.xlarge in different AZs)
	ris, err := promClient.QueryReservedInstances(ctx, "")
	if err != nil {
		t.Fatalf("failed to query reserved instances: %v", err)
	}
	if len(ris) != 2 {
		t.Fatalf("expected 2 RI entries, got %d", len(ris))
	}

	// Aggregate RIs by instance type
	aggByType := overlay.AggregateReservedInstances(ris)

	// Verify we have 1 instance type: m5.xlarge
	if len(aggByType) != 1 {
		t.Fatalf("expected 1 instance type, got %d", len(aggByType))
	}

	// Test m5.xlarge aggregation (2 RIs in different AZs) - now keyed by "instanceType:region"
	riKey := "m5.xlarge:us-west-2"
	m5XlargeAgg, ok := aggByType[riKey]
	if !ok {
		keys := make([]string, 0, len(aggByType))
		for k := range aggByType {
			keys = append(keys, k)
		}
		t.Fatalf("expected %s in aggregation, got keys: %v", riKey, keys)
	}

	// Expected total count: 3 (us-west-2a) + 2 (us-west-2b) = 5 RIs
	expectedCount := 5
	if m5XlargeAgg.TotalCount != expectedCount {
		t.Errorf("expected TotalCount=%d, got %d", expectedCount, m5XlargeAgg.TotalCount)
	}

	// Create ONE decision from aggregated data
	decision := engine.AnalyzeReservedInstance(m5XlargeAgg)

	if decision.Name != "cost-aware-ri-m5.xlarge-us-west-2" {
		t.Errorf("expected Name='cost-aware-ri-m5.xlarge-us-west-2', got '%s'", decision.Name)
	}

	if !decision.ShouldExist {
		t.Errorf("expected ShouldExist=true, got false. Reason: %s", decision.Reason)
	}

	// Verify the reason mentions the aggregated count
	expectedReason := "5 reserved instances available"
	if decision.Reason != expectedReason {
		t.Errorf("expected Reason='%s', got '%s'", expectedReason, decision.Reason)
	}

	t.Logf("SUCCESS: Created 1 aggregated decision with count=%d RIs", m5XlargeAgg.TotalCount)
}
