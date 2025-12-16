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

package prometheus

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/nextdoor/karve/internal/testutil"
)

func TestNewClient(t *testing.T) {
	server := testutil.NewMockPrometheusServer()
	defer server.Close()

	client, err := NewClient(server.URL, "123456789012", "us-west-2", logr.Discard())
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}

	if client == nil {
		t.Error("NewClient() returned nil client")
	}
}

func TestNewClient_InvalidURL(t *testing.T) {
	// Invalid URL scheme should still succeed (Prometheus client accepts it)
	_, err := NewClient("not-a-url", "123456789012", "us-west-2", logr.Discard())
	if err != nil {
		t.Errorf("NewClient() with invalid URL failed: %v", err)
	}
}

func TestQuerySavingsPlanCapacity(t *testing.T) {
	server := testutil.NewMockPrometheusServer()
	defer server.Close()

	server.SetMetrics(testutil.LuminaMetricsWithSPCapacity())

	client, err := NewClient(server.URL, "123456789012", "us-west-2", logr.Discard())
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}

	ctx := context.Background()

	tests := []struct {
		name           string
		instanceFamily string
		wantCount      int
		wantCapacity   float64
	}{
		{
			name:           "m5 family",
			instanceFamily: "m5",
			wantCount:      1,
			wantCapacity:   50.00,
		},
		{
			name:           "all families",
			instanceFamily: "",
			wantCount:      2, // m5 + c5
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capacities, err := client.QuerySavingsPlanCapacity(ctx, tt.instanceFamily)
			if err != nil {
				t.Fatalf("QuerySavingsPlanCapacity() error = %v", err)
			}

			if len(capacities) != tt.wantCount {
				t.Errorf("got %d capacities, want %d", len(capacities), tt.wantCount)
			}

			if tt.wantCount > 0 && tt.wantCapacity > 0 {
				if capacities[0].RemainingCapacity != tt.wantCapacity {
					t.Errorf("got capacity %f, want %f", capacities[0].RemainingCapacity, tt.wantCapacity)
				}
			}
		})
	}
}

func TestQuerySavingsPlanCapacity_NoCapacity(t *testing.T) {
	server := testutil.NewMockPrometheusServer()
	defer server.Close()

	server.SetMetrics(testutil.LuminaMetricsWithNoCapacity())

	client, _ := NewClient(server.URL, "123456789012", "us-west-2", logr.Discard())
	ctx := context.Background()

	capacities, err := client.QuerySavingsPlanCapacity(ctx, "m5")
	if err != nil {
		t.Fatalf("QuerySavingsPlanCapacity() error = %v", err)
	}

	if len(capacities) != 1 {
		t.Fatalf("expected 1 result, got %d", len(capacities))
	}

	// Should have 0 capacity
	if capacities[0].RemainingCapacity != 0.0 {
		t.Errorf("expected 0 capacity, got %f", capacities[0].RemainingCapacity)
	}
}

func TestQuerySavingsPlanCapacity_Empty(t *testing.T) {
	server := testutil.NewMockPrometheusServer()
	defer server.Close()

	server.SetMetrics(testutil.LuminaMetricsEmpty())

	client, _ := NewClient(server.URL, "123456789012", "us-west-2", logr.Discard())
	ctx := context.Background()

	capacities, err := client.QuerySavingsPlanCapacity(ctx, "m5")
	if err != nil {
		t.Fatalf("QuerySavingsPlanCapacity() error = %v", err)
	}

	if len(capacities) != 0 {
		t.Errorf("expected 0 results for empty metrics, got %d", len(capacities))
	}
}

func TestQueryReservedInstances(t *testing.T) {
	server := testutil.NewMockPrometheusServer()
	defer server.Close()

	server.SetMetrics(testutil.LuminaMetricsWithSPCapacity())

	client, _ := NewClient(server.URL, "123456789012", "us-west-2", logr.Discard())
	ctx := context.Background()

	tests := []struct {
		name         string
		instanceType string
		wantCount    int
	}{
		{
			name:         "specific instance type",
			instanceType: "m5.xlarge",
			wantCount:    1,
		},
		{
			name:         "all instance types",
			instanceType: "",
			wantCount:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ris, err := client.QueryReservedInstances(ctx, tt.instanceType)
			if err != nil {
				t.Fatalf("QueryReservedInstances() error = %v", err)
			}

			if len(ris) != tt.wantCount {
				t.Errorf("got %d RIs, want %d", len(ris), tt.wantCount)
			}

			if len(ris) > 0 {
				if ris[0].InstanceType == "" {
					t.Error("RI missing instance type")
				}
				if ris[0].Region == "" {
					t.Error("RI missing region")
				}
			}
		})
	}
}

func TestQueryReservedInstances_Empty(t *testing.T) {
	server := testutil.NewMockPrometheusServer()
	defer server.Close()

	server.SetMetrics(testutil.LuminaMetricsWithNoCapacity())

	client, _ := NewClient(server.URL, "123456789012", "us-west-2", logr.Discard())
	ctx := context.Background()

	ris, err := client.QueryReservedInstances(ctx, "m5.xlarge")
	if err != nil {
		t.Fatalf("QueryReservedInstances() error = %v", err)
	}

	if len(ris) != 0 {
		t.Errorf("expected 0 RIs when capacity exhausted, got %d", len(ris))
	}
}

func TestQuerySpotPrice(t *testing.T) {
	server := testutil.NewMockPrometheusServer()
	defer server.Close()

	server.SetMetrics(testutil.LuminaMetricsWithSpotPrices())

	client, _ := NewClient(server.URL, "123456789012", "us-west-2", logr.Discard())
	ctx := context.Background()

	prices, err := client.QuerySpotPrice(ctx, "m5.xlarge")
	if err != nil {
		t.Fatalf("QuerySpotPrice() error = %v", err)
	}

	if len(prices) != 1 {
		t.Fatalf("expected 1 price, got %d", len(prices))
	}

	if prices[0].Price != 0.12 {
		t.Errorf("expected price 0.12, got %f", prices[0].Price)
	}

	if prices[0].InstanceType != "m5.xlarge" {
		t.Errorf("expected instance type m5.xlarge, got %s", prices[0].InstanceType)
	}
}

func TestQueryOnDemandPrice(t *testing.T) {
	server := testutil.NewMockPrometheusServer()
	defer server.Close()

	server.SetMetrics(testutil.LuminaMetricsWithSpotPrices())

	client, _ := NewClient(server.URL, "123456789012", "us-west-2", logr.Discard())
	ctx := context.Background()

	prices, err := client.QueryOnDemandPrice(ctx, "m5.xlarge")
	if err != nil {
		t.Fatalf("QueryOnDemandPrice() error = %v", err)
	}

	if len(prices) != 1 {
		t.Fatalf("expected 1 price, got %d", len(prices))
	}

	if prices[0].Price != 0.192 {
		t.Errorf("expected price 0.192, got %f", prices[0].Price)
	}

	if prices[0].OperatingSystem != "Linux" {
		t.Errorf("expected OS Linux, got %s", prices[0].OperatingSystem)
	}
}

func TestDataFreshness(t *testing.T) {
	server := testutil.NewMockPrometheusServer()
	defer server.Close()

	// Add data freshness metric
	server.SetMetrics(testutil.MetricFixture{
		`lumina_data_freshness_seconds`: `{
			"status": "success",
			"data": {
				"resultType": "vector",
				"result": [
					{
						"metric": {},
						"value": [1640000000, "45.5"]
					}
				]
			}
		}`,
	})

	client, _ := NewClient(server.URL, "123456789012", "us-west-2", logr.Discard())
	ctx := context.Background()

	freshness, err := client.DataFreshness(ctx)
	if err != nil {
		t.Fatalf("DataFreshness() error = %v", err)
	}

	if freshness != 45.5 {
		t.Errorf("expected freshness 45.5, got %f", freshness)
	}
}

func TestDataFreshness_NoMetric(t *testing.T) {
	server := testutil.NewMockPrometheusServer()
	defer server.Close()

	server.SetMetrics(testutil.LuminaMetricsEmpty())

	client, _ := NewClient(server.URL, "123456789012", "us-west-2", logr.Discard())
	ctx := context.Background()

	_, err := client.DataFreshness(ctx)
	if err == nil {
		t.Error("DataFreshness() expected error when metric missing, got nil")
	}
}

func TestQueryRaw(t *testing.T) {
	server := testutil.NewMockPrometheusServer()
	defer server.Close()

	server.SetMetrics(testutil.LuminaMetricsWithSPCapacity())

	client, _ := NewClient(server.URL, "123456789012", "us-west-2", logr.Discard())
	ctx := context.Background()

	result, err := client.QueryRaw(ctx, `savings_plan_remaining_capacity{type="ec2_instance",instance_family="m5"}`)
	if err != nil {
		t.Fatalf("QueryRaw() error = %v", err)
	}

	if result == "" {
		t.Error("QueryRaw() returned empty result")
	}
}

func TestParseFloat64(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    float64
		wantErr bool
	}{
		{
			name:    "valid float",
			input:   "123.45",
			want:    123.45,
			wantErr: false,
		},
		{
			name:    "integer",
			input:   "100",
			want:    100.0,
			wantErr: false,
		},
		{
			name:    "invalid",
			input:   "not-a-number",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFloat64(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseFloat64() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseFloat64() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestQueryWithPrometheusWarnings(t *testing.T) {
	server := testutil.NewMockPrometheusServer()
	defer server.Close()

	// Add metric that will trigger warnings path (though we can't easily mock warnings)
	server.SetMetrics(testutil.LuminaMetricsWithSPCapacity())

	client, _ := NewClient(server.URL, "123456789012", "us-west-2", logr.Discard())
	ctx := context.Background()

	// These queries should succeed even with warnings (which are logged and ignored)
	_, err := client.QuerySavingsPlanCapacity(ctx, "m5")
	if err != nil {
		t.Errorf("QuerySavingsPlanCapacity() with warnings failed: %v", err)
	}
}

func TestQueryServerUnavailable(t *testing.T) {
	// Use invalid server URL to trigger connection error
	client, _ := NewClient("http://localhost:1", "123456789012", "us-west-2", logr.Discard())
	ctx := context.Background()

	// All query methods should handle connection errors gracefully
	_, err := client.QuerySavingsPlanCapacity(ctx, "m5")
	if err == nil {
		t.Error("QuerySavingsPlanCapacity() expected error with unavailable server")
	}

	_, err = client.QueryReservedInstances(ctx, "m5.xlarge")
	if err == nil {
		t.Error("QueryReservedInstances() expected error with unavailable server")
	}

	_, err = client.QuerySpotPrice(ctx, "m5.xlarge")
	if err == nil {
		t.Error("QuerySpotPrice() expected error with unavailable server")
	}

	_, err = client.QueryOnDemandPrice(ctx, "m5.xlarge")
	if err == nil {
		t.Error("QueryOnDemandPrice() expected error with unavailable server")
	}

	_, err = client.DataFreshness(ctx)
	if err == nil {
		t.Error("DataFreshness() expected error with unavailable server")
	}

	_, err = client.QueryRaw(ctx, "test_metric")
	if err == nil {
		t.Error("QueryRaw() expected error with unavailable server")
	}
}

func TestQuerySavingsPlanUtilization(t *testing.T) {
	server := testutil.NewMockPrometheusServer()
	defer server.Close()

	server.SetMetrics(testutil.LuminaMetricsWithSPUtilization())

	client, err := NewClient(server.URL, "123456789012", "us-west-2", logr.Discard())
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}

	ctx := context.Background()

	tests := []struct {
		name                 string
		spType               string
		wantCount            int
		wantUtilizationFirst float64
	}{
		{
			name:                 "compute SPs",
			spType:               SavingsPlanTypeCompute,
			wantCount:            1,
			wantUtilizationFirst: 87.5,
		},
		{
			name:                 "ec2_instance SPs",
			spType:               SavingsPlanTypeEC2Instance,
			wantCount:            1,
			wantUtilizationFirst: 96.2,
		},
		{
			name:      "all SP types",
			spType:    "",
			wantCount: 2, // compute + ec2_instance
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			utilizations, err := client.QuerySavingsPlanUtilization(ctx, tt.spType)
			if err != nil {
				t.Fatalf("QuerySavingsPlanUtilization() error = %v", err)
			}

			if len(utilizations) != tt.wantCount {
				t.Errorf("got %d utilizations, want %d", len(utilizations), tt.wantCount)
			}

			if tt.wantCount > 0 && tt.wantUtilizationFirst > 0 {
				if utilizations[0].UtilizationPercent != tt.wantUtilizationFirst {
					t.Errorf("got utilization %f%%, want %f%%", utilizations[0].UtilizationPercent, tt.wantUtilizationFirst)
				}
			}
		})
	}
}

func TestQuerySavingsPlanUtilization_Empty(t *testing.T) {
	server := testutil.NewMockPrometheusServer()
	defer server.Close()

	// No metrics loaded - should return empty result
	client, err := NewClient(server.URL, "123456789012", "us-west-2", logr.Discard())
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}

	ctx := context.Background()
	utilizations, err := client.QuerySavingsPlanUtilization(ctx, "")
	if err != nil {
		t.Fatalf("QuerySavingsPlanUtilization() error = %v", err)
	}

	if len(utilizations) != 0 {
		t.Errorf("got %d utilizations, want 0", len(utilizations))
	}
}
