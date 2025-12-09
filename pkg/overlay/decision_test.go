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

package overlay

import (
	"testing"

	"github.com/nextdoor/karve/pkg/config"
	"github.com/nextdoor/karve/pkg/prometheus"
)

const (
	// Expected price value for all test cases (overlay price is not yet implemented)
	expectedTestPrice = "0.00"
)

// Helper to create test config with default values
func testConfig() *config.Config {
	return &config.Config{
		OverlayManagement: config.OverlaysConfig{
			UtilizationThreshold: 95.0,
			Weights: config.OverlayWeightsConfig{
				ReservedInstance:       30,
				EC2InstanceSavingsPlan: 20,
				ComputeSavingsPlan:     10,
			},
		},
	}
}

func TestAnalyzeComputeSavingsPlan(t *testing.T) {
	engine := NewDecisionEngine(testConfig())

	tests := []struct {
		name               string
		utilization        prometheus.SavingsPlanUtilization
		capacity           prometheus.SavingsPlanCapacity
		wantShouldExist    bool
		wantReasonContains string
	}{
		{
			name: "below threshold with capacity - should exist",
			utilization: prometheus.SavingsPlanUtilization{
				Type:               prometheus.SavingsPlanTypeCompute,
				UtilizationPercent: 85.0,
			},
			capacity: prometheus.SavingsPlanCapacity{
				Type:              prometheus.SavingsPlanTypeCompute,
				RemainingCapacity: 50.00,
			},
			wantShouldExist:    true,
			wantReasonContains: "below threshold",
		},
		{
			name: "at threshold - should not exist",
			utilization: prometheus.SavingsPlanUtilization{
				Type:               prometheus.SavingsPlanTypeCompute,
				UtilizationPercent: 95.0,
			},
			capacity: prometheus.SavingsPlanCapacity{
				Type:              prometheus.SavingsPlanTypeCompute,
				RemainingCapacity: 10.00,
			},
			wantShouldExist:    false,
			wantReasonContains: "at/above threshold",
		},
		{
			name: "above threshold - should not exist",
			utilization: prometheus.SavingsPlanUtilization{
				Type:               prometheus.SavingsPlanTypeCompute,
				UtilizationPercent: 110.0,
			},
			capacity: prometheus.SavingsPlanCapacity{
				Type:              prometheus.SavingsPlanTypeCompute,
				RemainingCapacity: -5.00,
			},
			wantShouldExist:    false,
			wantReasonContains: "at/above threshold",
		},
		{
			name: "below threshold but no capacity - should not exist",
			utilization: prometheus.SavingsPlanUtilization{
				Type:               prometheus.SavingsPlanTypeCompute,
				UtilizationPercent: 80.0,
			},
			capacity: prometheus.SavingsPlanCapacity{
				Type:              prometheus.SavingsPlanTypeCompute,
				RemainingCapacity: 0.00,
			},
			wantShouldExist:    false,
			wantReasonContains: "no remaining capacity",
		},
		{
			name: "zero utilization with capacity - should exist",
			utilization: prometheus.SavingsPlanUtilization{
				Type:               prometheus.SavingsPlanTypeCompute,
				UtilizationPercent: 0.0,
			},
			capacity: prometheus.SavingsPlanCapacity{
				Type:              prometheus.SavingsPlanTypeCompute,
				RemainingCapacity: 100.00,
			},
			wantShouldExist:    true,
			wantReasonContains: "below threshold",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := engine.AnalyzeComputeSavingsPlanSingle(tt.utilization, tt.capacity)

			// Verify decision fields
			if decision.Name != "cost-aware-compute-sp-global" {
				t.Errorf("Name = %q, want %q", decision.Name, "cost-aware-compute-sp-global")
			}

			if decision.CapacityType != CapacityTypeComputeSavingsPlan {
				t.Errorf("CapacityType = %q, want %q", decision.CapacityType, CapacityTypeComputeSavingsPlan)
			}

			if decision.Weight != 10 {
				t.Errorf("Weight = %d, want 10", decision.Weight)
			}

			if decision.Price != expectedTestPrice {
				t.Errorf("Price = %q, want %q", decision.Price, expectedTestPrice)
			}

			if decision.ShouldExist != tt.wantShouldExist {
				t.Errorf("ShouldExist = %v, want %v", decision.ShouldExist, tt.wantShouldExist)
			}

			if tt.wantReasonContains != "" {
				if decision.Reason == "" {
					t.Errorf("Reason is empty, want it to contain %q", tt.wantReasonContains)
				}
				// Simple substring check
				if len(decision.Reason) > 0 && !contains(decision.Reason, tt.wantReasonContains) {
					t.Errorf("Reason = %q, want it to contain %q", decision.Reason, tt.wantReasonContains)
				}
			}

			// Use epsilon comparison for floating point values to avoid precision issues
			const epsilon = 1e-9
			if diff := decision.UtilizationPercent - tt.utilization.UtilizationPercent; diff < -epsilon || diff > epsilon {
				t.Errorf(
					"UtilizationPercent = %f, want %f (diff: %e)",
					decision.UtilizationPercent,
					tt.utilization.UtilizationPercent,
					diff,
				)
			}

			if diff := decision.RemainingCapacity - tt.capacity.RemainingCapacity; diff < -epsilon || diff > epsilon {
				t.Errorf(
					"RemainingCapacity = %f, want %f (diff: %e)",
					decision.RemainingCapacity,
					tt.capacity.RemainingCapacity,
					diff,
				)
			}
		})
	}
}

func TestAnalyzeEC2InstanceSavingsPlan(t *testing.T) {
	engine := NewDecisionEngine(testConfig())

	tests := []struct {
		name               string
		utilization        prometheus.SavingsPlanUtilization
		capacity           prometheus.SavingsPlanCapacity
		wantName           string
		wantShouldExist    bool
		wantReasonContains string
	}{
		{
			name: "m5 family below threshold with capacity - should exist",
			utilization: prometheus.SavingsPlanUtilization{
				Type:               prometheus.SavingsPlanTypeEC2Instance,
				InstanceFamily:     "m5",
				Region:             "us-west-2",
				UtilizationPercent: 75.0,
			},
			capacity: prometheus.SavingsPlanCapacity{
				Type:              prometheus.SavingsPlanTypeEC2Instance,
				InstanceFamily:    "m5",
				Region:            "us-west-2",
				RemainingCapacity: 25.00,
			},
			wantName:           "cost-aware-ec2-sp-m5-us-west-2",
			wantShouldExist:    true,
			wantReasonContains: "below threshold",
		},
		{
			name: "c5 family at threshold - should not exist",
			utilization: prometheus.SavingsPlanUtilization{
				Type:               prometheus.SavingsPlanTypeEC2Instance,
				InstanceFamily:     "c5",
				Region:             "us-east-1",
				UtilizationPercent: 95.0,
			},
			capacity: prometheus.SavingsPlanCapacity{
				Type:              prometheus.SavingsPlanTypeEC2Instance,
				InstanceFamily:    "c5",
				Region:            "us-east-1",
				RemainingCapacity: 5.00,
			},
			wantName:           "cost-aware-ec2-sp-c5-us-east-1",
			wantShouldExist:    false,
			wantReasonContains: "at/above threshold",
		},
		{
			name: "r6i family below threshold but no capacity - should not exist",
			utilization: prometheus.SavingsPlanUtilization{
				Type:               prometheus.SavingsPlanTypeEC2Instance,
				InstanceFamily:     "r6i",
				Region:             "eu-west-1",
				UtilizationPercent: 88.0,
			},
			capacity: prometheus.SavingsPlanCapacity{
				Type:              prometheus.SavingsPlanTypeEC2Instance,
				InstanceFamily:    "r6i",
				Region:            "eu-west-1",
				RemainingCapacity: 0.00,
			},
			wantName:           "cost-aware-ec2-sp-r6i-eu-west-1",
			wantShouldExist:    false,
			wantReasonContains: "no remaining capacity",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := engine.AnalyzeEC2InstanceSavingsPlanSingle(tt.utilization, tt.capacity)

			if decision.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", decision.Name, tt.wantName)
			}

			if decision.CapacityType != CapacityTypeEC2InstanceSavingsPlan {
				t.Errorf("CapacityType = %q, want %q", decision.CapacityType, CapacityTypeEC2InstanceSavingsPlan)
			}

			if decision.Weight != 20 {
				t.Errorf("Weight = %d, want 20", decision.Weight)
			}

			if decision.Price != expectedTestPrice {
				t.Errorf("Price = %q, want %q", decision.Price, expectedTestPrice)
			}

			if decision.ShouldExist != tt.wantShouldExist {
				t.Errorf("ShouldExist = %v, want %v", decision.ShouldExist, tt.wantShouldExist)
			}

			if tt.wantReasonContains != "" && !contains(decision.Reason, tt.wantReasonContains) {
				t.Errorf("Reason = %q, want it to contain %q", decision.Reason, tt.wantReasonContains)
			}
		})
	}
}

func TestAnalyzeReservedInstance(t *testing.T) {
	engine := NewDecisionEngine(testConfig())

	tests := []struct {
		name               string
		ri                 prometheus.ReservedInstance
		wantName           string
		wantShouldExist    bool
		wantReasonContains string
	}{
		{
			name: "RI available - should exist",
			ri: prometheus.ReservedInstance{
				InstanceType: "m5.xlarge",
				Region:       "us-west-2",
				Count:        5,
			},
			wantName:           "cost-aware-ri-m5.xlarge-us-west-2",
			wantShouldExist:    true,
			wantReasonContains: "reserved instances available",
		},
		{
			name: "RI not available - should not exist",
			ri: prometheus.ReservedInstance{
				InstanceType: "c5.large",
				Region:       "us-east-1",
				Count:        0,
			},
			wantName:           "cost-aware-ri-c5.large-us-east-1",
			wantShouldExist:    false,
			wantReasonContains: "no reserved instances available",
		},
		{
			name: "single RI - should exist",
			ri: prometheus.ReservedInstance{
				InstanceType: "r6i.2xlarge",
				Region:       "eu-west-1",
				Count:        1,
			},
			wantName:           "cost-aware-ri-r6i.2xlarge-eu-west-1",
			wantShouldExist:    true,
			wantReasonContains: "reserved instances available",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := engine.AnalyzeReservedInstanceSingle(tt.ri)

			if decision.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", decision.Name, tt.wantName)
			}

			if decision.CapacityType != CapacityTypeReservedInstance {
				t.Errorf("CapacityType = %q, want %q", decision.CapacityType, CapacityTypeReservedInstance)
			}

			if decision.Weight != 30 {
				t.Errorf("Weight = %d, want 30", decision.Weight)
			}

			if decision.Price != expectedTestPrice {
				t.Errorf("Price = %q, want %q", decision.Price, expectedTestPrice)
			}

			if decision.ShouldExist != tt.wantShouldExist {
				t.Errorf("ShouldExist = %v, want %v", decision.ShouldExist, tt.wantShouldExist)
			}

			if !contains(decision.Reason, tt.wantReasonContains) {
				t.Errorf("Reason = %q, want it to contain %q", decision.Reason, tt.wantReasonContains)
			}
		})
	}
}

func TestDecisionEngineWithCustomThreshold(t *testing.T) {
	// Test with custom threshold of 90%
	cfg := testConfig()
	cfg.Overlays.UtilizationThreshold = 90.0
	engine := NewDecisionEngine(cfg)

	utilization := prometheus.SavingsPlanUtilization{
		Type:               prometheus.SavingsPlanTypeCompute,
		UtilizationPercent: 92.0, // Above custom threshold
	}
	capacity := prometheus.SavingsPlanCapacity{
		Type:              prometheus.SavingsPlanTypeCompute,
		RemainingCapacity: 10.00,
	}

	decision := engine.AnalyzeComputeSavingsPlanSingle(utilization, capacity)

	if decision.ShouldExist {
		t.Errorf("ShouldExist = true with 92%% utilization and 90%% threshold, want false")
	}

	if !contains(decision.Reason, "at/above threshold 90") {
		t.Errorf("Reason = %q, want it to mention threshold 90", decision.Reason)
	}
}

func TestDecisionEngineWithCustomWeights(t *testing.T) {
	cfg := testConfig()
	cfg.Overlays.Weights.ReservedInstance = 100
	cfg.Overlays.Weights.EC2InstanceSavingsPlan = 50
	cfg.Overlays.Weights.ComputeSavingsPlan = 25
	engine := NewDecisionEngine(cfg)

	// Test RI weight
	ri := prometheus.ReservedInstance{
		InstanceType: "m5.xlarge",
		Count:        1,
	}
	riDecision := engine.AnalyzeReservedInstanceSingle(ri)
	if riDecision.Weight != 100 {
		t.Errorf("RI Weight = %d, want 100", riDecision.Weight)
	}

	// Test EC2 Instance SP weight
	ec2SPUtil := prometheus.SavingsPlanUtilization{
		Type:               prometheus.SavingsPlanTypeEC2Instance,
		InstanceFamily:     "m5",
		UtilizationPercent: 80.0,
	}
	ec2SPCap := prometheus.SavingsPlanCapacity{
		Type:              prometheus.SavingsPlanTypeEC2Instance,
		InstanceFamily:    "m5",
		RemainingCapacity: 20.00,
	}
	ec2SPDecision := engine.AnalyzeEC2InstanceSavingsPlanSingle(ec2SPUtil, ec2SPCap)
	if ec2SPDecision.Weight != 50 {
		t.Errorf("EC2 SP Weight = %d, want 50", ec2SPDecision.Weight)
	}

	// Test Compute SP weight
	computeSPUtil := prometheus.SavingsPlanUtilization{
		Type:               prometheus.SavingsPlanTypeCompute,
		UtilizationPercent: 80.0,
	}
	computeSPCap := prometheus.SavingsPlanCapacity{
		Type:              prometheus.SavingsPlanTypeCompute,
		RemainingCapacity: 50.00,
	}
	computeSPDecision := engine.AnalyzeComputeSavingsPlanSingle(computeSPUtil, computeSPCap)
	if computeSPDecision.Weight != 25 {
		t.Errorf("Compute SP Weight = %d, want 25", computeSPDecision.Weight)
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && stringContains(s, substr))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
