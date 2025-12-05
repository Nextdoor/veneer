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

// Package testutil provides testing utilities for Karve, including mock Prometheus servers
// and test fixtures for Lumina metrics.
//
// This package is designed to simulate Lumina's output without requiring a running Lumina
// instance or real Prometheus server during tests.
package testutil

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
)

// MockPrometheusServer creates an in-memory HTTP server that responds to Prometheus API queries
// with predefined metric data. This allows testing Karve's Prometheus client without running
// actual Lumina or Prometheus instances.
//
// The server supports:
//   - /api/v1/query - Instant queries
//   - /api/v1/query_range - Range queries (returns same data as instant for simplicity)
//
// Usage:
//
//	server := testutil.NewMockPrometheusServer()
//	server.SetMetrics(testutil.LuminaMetricsWithSPCapacity())
//	defer server.Close()
//
//	// Use server.URL in your Prometheus client
//	client := prometheus.NewClient(server.URL)
type MockPrometheusServer struct {
	*httptest.Server
	metrics map[string]string // query -> response JSON
}

// NewMockPrometheusServer creates a new mock Prometheus server with no metrics loaded.
// Use SetMetrics() to load test data before making queries.
func NewMockPrometheusServer() *MockPrometheusServer {
	mock := &MockPrometheusServer{
		metrics: make(map[string]string),
	}

	// Create HTTP server with handler
	mock.Server = httptest.NewServer(http.HandlerFunc(mock.handler))

	return mock
}

// SetMetrics loads metric fixtures into the mock server.
// The fixtures should be created using the MetricFixture functions below.
//
// Example:
//
//	server.SetMetrics(
//	    testutil.LuminaMetricsWithSPCapacity(),
//	    testutil.LuminaMetricsWithNoCapacity(),
//	)
func (m *MockPrometheusServer) SetMetrics(fixtures ...MetricFixture) {
	for _, fixture := range fixtures {
		for query, response := range fixture {
			m.metrics[query] = response
		}
	}
}

// ClearMetrics removes all loaded metrics from the server.
// Useful for resetting state between tests.
func (m *MockPrometheusServer) ClearMetrics() {
	m.metrics = make(map[string]string)
}

// handler processes Prometheus API requests and returns mocked responses.
// Supports both instant queries (/api/v1/query) and range queries (/api/v1/query_range).
func (m *MockPrometheusServer) handler(w http.ResponseWriter, r *http.Request) {
	// Parse query parameter - handle both GET (query param) and POST (form body)
	var query string
	if r.Method == http.MethodPost {
		// For POST requests, the Prometheus client sends form-encoded data
		if err := r.ParseForm(); err != nil {
			http.Error(w, `{"status":"error","errorType":"bad_data","error":"failed to parse form"}`, http.StatusBadRequest)
			return
		}
		query = r.FormValue("query")
	} else {
		// For GET requests, query is in URL parameter
		query = r.URL.Query().Get("query")
	}
	if query == "" {
		http.Error(w, `{"status":"error","errorType":"bad_data","error":"query missing"}`, http.StatusBadRequest)
		return
	}

	// Normalize query (remove extra whitespace for matching)
	query = strings.TrimSpace(query)

	// Look up response
	response, ok := m.metrics[query]
	if !ok {
		// Return empty result set if query not found (not an error)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
		return
	}

	// Return mocked response
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, response)
}

// MetricFixture represents a set of Prometheus queries and their responses.
// Keys are PromQL queries, values are JSON responses in Prometheus API format.
type MetricFixture map[string]string

// LuminaMetricsWithSPCapacity returns metrics showing available Savings Plans capacity.
// Scenario: m5 family has $50/hour remaining capacity, c5 has $30/hour remaining.
//
// Use this fixture when testing the "prefer RI/SP" decision path.
func LuminaMetricsWithSPCapacity() MetricFixture {
	return MetricFixture{
		// Query with instance_family selector
		`savings_plan_remaining_capacity{instance_family="m5"}`: `{
			"status": "success",
			"data": {
				"resultType": "vector",
				"result": [
					{
						"metric": {
							"type": "ec2_instance",
							"instance_family": "m5",
							"savings_plan_arn": "arn:aws:savingsplans::123456789012:savingsplan/sp-12345",
							"account_id": "123456789012"
						},
						"value": [1640000000, "50.00"]
					}
				]
			}
		}`,

		// Query without selector (all families)
		`savings_plan_remaining_capacity`: `{
			"status": "success",
			"data": {
				"resultType": "vector",
				"result": [
					{
						"metric": {
							"type": "ec2_instance",
							"instance_family": "m5",
							"savings_plan_arn": "arn:aws:savingsplans::123456789012:savingsplan/sp-12345",
							"account_id": "123456789012"
						},
						"value": [1640000000, "50.00"]
					},
					{
						"metric": {
							"type": "compute",
							"instance_family": "c5",
							"savings_plan_arn": "arn:aws:savingsplans::123456789012:savingsplan/sp-67890",
							"account_id": "123456789012"
						},
						"value": [1640000000, "30.00"]
					}
				]
			}
		}`,

		// Old query format (kept for backwards compat)
		`savings_plan_remaining_capacity{type="ec2_instance",instance_family="m5"}`: `{
			"status": "success",
			"data": {
				"resultType": "vector",
				"result": [
					{
						"metric": {
							"type": "ec2_instance",
							"instance_family": "m5",
							"savings_plan_arn": "arn:aws:savingsplans::123456789012:savingsplan/sp-12345",
							"account_id": "123456789012"
						},
						"value": [1640000000, "50.00"]
					}
				]
			}
		}`,

		// Query: savings_plan_remaining_capacity{type="compute"}
		`savings_plan_remaining_capacity{type="compute"}`: `{
			"status": "success",
			"data": {
				"resultType": "vector",
				"result": [
					{
						"metric": {
							"type": "compute",
							"instance_family": "c5",
							"savings_plan_arn": "arn:aws:savingsplans::123456789012:savingsplan/sp-67890",
							"account_id": "123456789012"
						},
						"value": [1640000000, "30.00"]
					}
				]
			}
		}`,

		// Query: ec2_reserved_instance (all instance types)
		`ec2_reserved_instance`: `{
			"status": "success",
			"data": {
				"resultType": "vector",
				"result": [
					{
						"metric": {
							"account_id": "123456789012",
							"region": "us-west-2",
							"instance_type": "m5.xlarge",
							"availability_zone": "us-west-2a"
						},
						"value": [1640000000, "1"]
					}
				]
			}
		}`,

		// Query: ec2_reserved_instance{instance_type="m5.xlarge"}
		`ec2_reserved_instance{instance_type="m5.xlarge"}`: `{
			"status": "success",
			"data": {
				"resultType": "vector",
				"result": [
					{
						"metric": {
							"account_id": "123456789012",
							"region": "us-west-2",
							"instance_type": "m5.xlarge",
							"availability_zone": "us-west-2a"
						},
						"value": [1640000000, "1"]
					}
				]
			}
		}`,
	}
}

// LuminaMetricsWithNoCapacity returns metrics showing exhausted Savings Plans capacity.
// Scenario: All SP capacity is fully utilized (0 remaining).
//
// Use this fixture when testing the "prefer spot" decision path.
func LuminaMetricsWithNoCapacity() MetricFixture {
	return MetricFixture{
		// Query with instance_family selector
		`savings_plan_remaining_capacity{instance_family="m5"}`: `{
			"status": "success",
			"data": {
				"resultType": "vector",
				"result": [
					{
						"metric": {
							"type": "ec2_instance",
							"instance_family": "m5",
							"savings_plan_arn": "arn:aws:savingsplans::123456789012:savingsplan/sp-12345",
							"account_id": "123456789012"
						},
						"value": [1640000000, "0.00"]
					}
				]
			}
		}`,

		// Old format
		`savings_plan_remaining_capacity{type="ec2_instance",instance_family="m5"}`: `{
			"status": "success",
			"data": {
				"resultType": "vector",
				"result": [
					{
						"metric": {
							"type": "ec2_instance",
							"instance_family": "m5",
							"savings_plan_arn": "arn:aws:savingsplans::123456789012:savingsplan/sp-12345",
							"account_id": "123456789012"
						},
						"value": [1640000000, "0.00"]
					}
				]
			}
		}`,

		`ec2_reserved_instance`: `{
			"status": "success",
			"data": {
				"resultType": "vector",
				"result": []
			}
		}`,
	}
}

// LuminaMetricsEmpty returns empty metric results.
// Scenario: Lumina is running but has no data yet (initial startup).
//
// Use this fixture when testing error handling and edge cases.
func LuminaMetricsEmpty() MetricFixture {
	return MetricFixture{
		// With selector
		`savings_plan_remaining_capacity{instance_family="m5"}`: `{
			"status": "success",
			"data": {
				"resultType": "vector",
				"result": []
			}
		}`,

		// Old format
		`savings_plan_remaining_capacity{type="ec2_instance",instance_family="m5"}`: `{
			"status": "success",
			"data": {
				"resultType": "vector",
				"result": []
			}
		}`,

		`ec2_reserved_instance`: `{
			"status": "success",
			"data": {
				"resultType": "vector",
				"result": []
			}
		}`,
	}
}

// LuminaMetricsWithSpotPrices returns metrics including spot pricing data.
// Scenario: Spot prices available for m5 family instances.
//
// Use this fixture when testing cost comparison logic (spot vs RI/SP).
func LuminaMetricsWithSpotPrices() MetricFixture {
	return MetricFixture{
		// Spot pricing for m5.xlarge with selector
		`ec2_spot_price{instance_type="m5.xlarge"}`: `{
			"status": "success",
			"data": {
				"resultType": "vector",
				"result": [
					{
						"metric": {
							"instance_type": "m5.xlarge",
							"region": "us-west-2",
							"availability_zone": "us-west-2a"
						},
						"value": [1640000000, "0.12"]
					}
				]
			}
		}`,

		// Spot pricing (old format)
		`ec2_spot_price{instance_type="m5.xlarge",region="us-west-2"}`: `{
			"status": "success",
			"data": {
				"resultType": "vector",
				"result": [
					{
						"metric": {
							"instance_type": "m5.xlarge",
							"region": "us-west-2",
							"availability_zone": "us-west-2a"
						},
						"value": [1640000000, "0.12"]
					}
				]
			}
		}`,

		// On-demand pricing with selector
		`ec2_ondemand_price{instance_type="m5.xlarge"}`: `{
			"status": "success",
			"data": {
				"resultType": "vector",
				"result": [
					{
						"metric": {
							"instance_type": "m5.xlarge",
							"region": "us-west-2",
							"operating_system": "Linux"
						},
						"value": [1640000000, "0.192"]
					}
				]
			}
		}`,

		// On-demand pricing (old format)
		`ec2_ondemand_price{instance_type="m5.xlarge",region="us-west-2"}`: `{
			"status": "success",
			"data": {
				"resultType": "vector",
				"result": [
					{
						"metric": {
							"instance_type": "m5.xlarge",
							"region": "us-west-2",
							"operating_system": "Linux"
						},
						"value": [1640000000, "0.192"]
					}
				]
			}
		}`,
	}
}
