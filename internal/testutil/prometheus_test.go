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

package testutil

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"
)

func TestMockPrometheusServer(t *testing.T) {
	server := NewMockPrometheusServer()
	defer server.Close()

	// Load test metrics
	server.SetMetrics(LuminaMetricsWithSPCapacity())

	// Test querying loaded metric
	query := `savings_plan_remaining_capacity{type="ec2_instance",instance_family="m5"}`
	resp, err := http.Get(fmt.Sprintf("%s/api/v1/query?query=%s", server.URL, url.QueryEscape(query)))
	if err != nil {
		t.Fatalf("failed to query server: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("unexpected status code: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Verify response is valid JSON
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Check response structure
	status, ok := result["status"].(string)
	if !ok || status != "success" {
		t.Errorf("unexpected status: got %v, want 'success'", result["status"])
	}

	data, ok := result["data"].(map[string]interface{})
	if !ok {
		t.Fatal("response missing 'data' field")
	}

	results, ok := data["result"].([]interface{})
	if !ok {
		t.Fatal("response data missing 'result' array")
	}

	if len(results) != 1 {
		t.Errorf("unexpected result count: got %d, want 1", len(results))
	}
}

func TestMockPrometheusServer_UnknownQuery(t *testing.T) {
	server := NewMockPrometheusServer()
	defer server.Close()

	server.SetMetrics(LuminaMetricsWithSPCapacity())

	// Query that wasn't loaded
	query := "nonexistent_metric"
	resp, err := http.Get(fmt.Sprintf("%s/api/v1/query?query=%s", server.URL, url.QueryEscape(query)))
	if err != nil {
		t.Fatalf("failed to query server: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("unexpected status code: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Should return empty result set (not an error)
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	status, ok := result["status"].(string)
	if !ok || status != "success" {
		t.Errorf("unexpected status: got %v, want 'success'", result["status"])
	}

	data := result["data"].(map[string]interface{})
	results := data["result"].([]interface{})
	if len(results) != 0 {
		t.Errorf("expected empty results for unknown query, got %d results", len(results))
	}
}

func TestMockPrometheusServer_MissingQuery(t *testing.T) {
	server := NewMockPrometheusServer()
	defer server.Close()

	// No query parameter
	resp, err := http.Get(fmt.Sprintf("%s/api/v1/query", server.URL))
	if err != nil {
		t.Fatalf("failed to query server: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("unexpected status code: got %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestMockPrometheusServer_ClearMetrics(t *testing.T) {
	server := NewMockPrometheusServer()
	defer server.Close()

	// Load metrics
	server.SetMetrics(LuminaMetricsWithSPCapacity())

	// Verify metric exists
	query := `savings_plan_remaining_capacity{type="ec2_instance",instance_family="m5"}`
	resp, _ := http.Get(fmt.Sprintf("%s/api/v1/query?query=%s", server.URL, url.QueryEscape(query)))
	var result1 map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result1); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	resp.Body.Close()

	data1 := result1["data"].(map[string]interface{})
	results1 := data1["result"].([]interface{})
	if len(results1) != 1 {
		t.Errorf("expected 1 result before clear, got %d", len(results1))
	}

	// Clear metrics
	server.ClearMetrics()

	// Verify metric is gone
	resp2, _ := http.Get(fmt.Sprintf("%s/api/v1/query?query=%s", server.URL, url.QueryEscape(query)))
	var result2 map[string]interface{}
	if err := json.NewDecoder(resp2.Body).Decode(&result2); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	resp2.Body.Close()

	data2 := result2["data"].(map[string]interface{})
	results2 := data2["result"].([]interface{})
	if len(results2) != 0 {
		t.Errorf("expected 0 results after clear, got %d", len(results2))
	}
}

func TestLuminaMetricsFixtures(t *testing.T) {
	tests := []struct {
		name    string
		fixture MetricFixture
		query   string
		wantLen int // expected number of results
	}{
		{
			name:    "with SP capacity",
			fixture: LuminaMetricsWithSPCapacity(),
			query:   `savings_plan_remaining_capacity{type="ec2_instance",instance_family="m5"}`,
			wantLen: 1,
		},
		{
			name:    "no capacity",
			fixture: LuminaMetricsWithNoCapacity(),
			query:   `savings_plan_remaining_capacity{type="ec2_instance",instance_family="m5"}`,
			wantLen: 1, // Returns result with value "0.00"
		},
		{
			name:    "empty metrics",
			fixture: LuminaMetricsEmpty(),
			query:   `savings_plan_remaining_capacity{type="ec2_instance",instance_family="m5"}`,
			wantLen: 0, // Empty result array
		},
		{
			name:    "spot prices",
			fixture: LuminaMetricsWithSpotPrices(),
			query:   `ec2_spot_price{instance_type="m5.xlarge",region="us-west-2"}`,
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := NewMockPrometheusServer()
			defer server.Close()

			server.SetMetrics(tt.fixture)

			resp, err := http.Get(fmt.Sprintf("%s/api/v1/query?query=%s", server.URL, url.QueryEscape(tt.query)))
			if err != nil {
				t.Fatalf("failed to query server: %v", err)
			}
			defer resp.Body.Close()

			var result map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			data := result["data"].(map[string]interface{})
			results := data["result"].([]interface{})

			if len(results) != tt.wantLen {
				t.Errorf("unexpected result count: got %d, want %d", len(results), tt.wantLen)
			}
		})
	}
}
