# Test Utilities

This package provides testing utilities for Karve, including mock Prometheus servers and test fixtures for Lumina metrics.

## Mock Prometheus Server

The `MockPrometheusServer` allows testing Karve's Prometheus client without running actual Lumina or Prometheus instances.

### Basic Usage

```go
package mypackage

import (
    "testing"
    "github.com/nextdoor/karve/internal/testutil"
)

func TestMyFunction(t *testing.T) {
    // Create mock server
    server := testutil.NewMockPrometheusServer()
    defer server.Close()

    // Load metrics fixtures
    server.SetMetrics(testutil.LuminaMetricsWithSPCapacity())

    // Use server.URL with your Prometheus client
    client := prometheus.NewClient(server.URL)

    // Query metrics as normal
    result, err := client.Query("savings_plan_remaining_capacity{instance_family=\"m5\"}")
    // ... assertions ...
}
```

### Available Fixtures

| Fixture | Description | Use Case |
|---------|-------------|----------|
| `LuminaMetricsWithSPCapacity()` | SP capacity available ($50/hr m5, $30/hr c5) | Test "prefer RI/SP" path |
| `LuminaMetricsWithNoCapacity()` | SP capacity exhausted (all 0) | Test "prefer spot" path |
| `LuminaMetricsEmpty()` | No metrics available | Test error handling |
| `LuminaMetricsWithSpotPrices()` | Spot pricing data | Test cost comparison |

### Multiple Fixtures

Load multiple fixtures to simulate complex scenarios:

```go
server.SetMetrics(
    testutil.LuminaMetricsWithSPCapacity(),
    testutil.LuminaMetricsWithSpotPrices(),
)
```

### Custom Fixtures

Create custom fixtures for specific test scenarios:

```go
customMetrics := testutil.MetricFixture{
    `my_custom_query`: `{
        "status": "success",
        "data": {
            "resultType": "vector",
            "result": [
                {
                    "metric": {"label": "value"},
                    "value": [1640000000, "123.45"]
                }
            ]
        }
    }`,
}

server.SetMetrics(customMetrics)
```

### Clearing Metrics

Reset server state between tests:

```go
server.ClearMetrics()
server.SetMetrics(testutil.LuminaMetricsWithNoCapacity())
```

## Metric Formats

All fixtures return data in Prometheus HTTP API format:

```json
{
  "status": "success",
  "data": {
    "resultType": "vector",
    "result": [
      {
        "metric": {
          "type": "ec2_instance",
          "instance_family": "m5",
          "account_id": "123456789012"
        },
        "value": [1640000000, "50.00"]
      }
    ]
  }
}
```

## Integration Testing Example

Complete example showing how to test cost decision logic:

```go
func TestCostDecision_PreferRISP(t *testing.T) {
    // Setup mock Prometheus with available capacity
    server := testutil.NewMockPrometheusServer()
    defer server.Close()

    server.SetMetrics(
        testutil.LuminaMetricsWithSPCapacity(),
        testutil.LuminaMetricsWithSpotPrices(),
    )

    // Create controller with mock Prometheus URL
    cfg := &config.Config{
        PrometheusURL: server.URL,
    }

    controller := NewCostController(cfg)

    // Execute decision logic
    decision, err := controller.MakeDecision("m5")
    if err != nil {
        t.Fatalf("MakeDecision failed: %v", err)
    }

    // Verify decision prefers RI/SP due to available capacity
    if decision != PreferRISP {
        t.Errorf("expected PreferRISP, got %v", decision)
    }
}
```

## E2E Testing

For E2E tests, use the same mock server but run in a test cluster:

```go
func TestE2E_NodeOverlayCreation(t *testing.T) {
    // Create mock server
    server := testutil.NewMockPrometheusServer()
    defer server.Close()
    server.SetMetrics(testutil.LuminaMetricsWithSPCapacity())

    // Deploy Karve with mock Prometheus URL
    helmArgs := fmt.Sprintf(
        "--set prometheusUrl=%s",
        server.URL,
    )

    // ... deploy to test cluster and verify NodeOverlay creation ...
}
```

## Design Rationale

This approach was chosen over config-based test data because:

1. **Tests actual code paths**: The Prometheus client code is exercised in tests
2. **No production pollution**: Test data doesn't leak into production config
3. **Flexible scenarios**: Easy to create complex multi-query scenarios
4. **Realistic**: Simulates actual Prometheus HTTP API responses
5. **Reusable**: Same fixtures work for unit, integration, and E2E tests

Compare to Lumina's approach where test data goes in config YAML - that works for AWS APIs but not well for HTTP-based metric queries.
