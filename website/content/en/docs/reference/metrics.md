---
title: "Metrics"
description: "Prometheus metrics exposed by the Veneer controller."
weight: 20
---

Veneer exposes Prometheus metrics on the metrics endpoint (default `:8080/metrics`, configurable via [`metricsBindAddress`]({{< relref "configuration" >}})). All metrics use the `veneer_` namespace prefix.

Veneer intentionally does **not** duplicate Lumina metrics (which are already in Prometheus). Instead, it focuses on what Veneer decided and what actions it took.

## Metrics at a Glance

| Metric | Type | Description |
|--------|------|-------------|
| [`veneer_reconciliation_duration_seconds`](#reconciliation-metrics) | Histogram | Duration of reconciliation cycles |
| [`veneer_reconciliation_total`](#reconciliation-metrics) | Counter | Total reconciliation cycles |
| [`veneer_lumina_data_freshness_seconds`](#data-source-health-metrics) | Gauge | Age of Lumina data |
| [`veneer_lumina_data_available`](#data-source-health-metrics) | Gauge | Whether Lumina data is fresh |
| [`veneer_decision_total`](#decision-metrics) | Counter | Decisions made by the engine |
| [`veneer_reserved_instance_data_available`](#reserved-instance-metrics) | Gauge | Whether RI metrics are available |
| [`veneer_reserved_instance_count`](#reserved-instance-metrics) | Gauge | RI count by type and region |
| [`veneer_savings_plan_utilization_percent`](#savings-plan-metrics) | Gauge | SP utilization percentage |
| [`veneer_savings_plan_remaining_capacity_dollars`](#savings-plan-metrics) | Gauge | SP remaining capacity ($/hr) |
| [`veneer_overlay_operations_total`](#nodeoverlay-lifecycle-metrics) | Counter | Total overlay operations |
| [`veneer_overlay_operation_errors_total`](#nodeoverlay-lifecycle-metrics) | Counter | Total overlay operation errors |
| [`veneer_overlay_count`](#nodeoverlay-lifecycle-metrics) | Gauge | Current overlay count |
| [`veneer_prometheus_query_duration_seconds`](#prometheus-query-metrics) | Histogram | Prometheus query duration |
| [`veneer_prometheus_query_errors_total`](#prometheus-query-metrics) | Counter | Prometheus query errors |
| [`veneer_prometheus_query_result_count`](#prometheus-query-metrics) | Gauge | Prometheus query result count |
| [`veneer_config_overlays_disabled`](#configuration-metrics) | Gauge | Whether overlays are disabled |
| [`veneer_config_utilization_threshold_percent`](#configuration-metrics) | Gauge | Configured utilization threshold |
| [`veneer_info`](#info-metric) | Gauge | Controller version info |

## Reconciliation Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `veneer_reconciliation_duration_seconds` | Histogram | -- | Duration of metrics reconciliation cycles. Buckets: 0.1s to ~51s (exponential). |
| `veneer_reconciliation_total` | Counter | `result` | Total number of reconciliation cycles. Labels: `result=success\|error`. |

## Data Source Health Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `veneer_lumina_data_freshness_seconds` | Gauge | -- | Age of Lumina data in seconds. |
| `veneer_lumina_data_available` | Gauge | -- | `1` if Lumina data is available and fresh, `0` if stale or unavailable. |

## Decision Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `veneer_decision_total` | Counter | `capacity_type`, `should_exist`, `reason` | Total decisions made by the decision engine. |

**Label values for `veneer_decision_total`:**

| Label | Values | Description |
|-------|--------|-------------|
| `capacity_type` | `compute_savings_plan`, `ec2_instance_savings_plan`, `reserved_instance`, `preference` | Type of AWS pre-paid capacity |
| `should_exist` | `true`, `false` | Whether an overlay should exist based on the decision |
| `reason` | `capacity_available`, `utilization_above_threshold`, `no_capacity`, `ri_available`, `ri_not_found`, `unknown` | Reason for the decision |

## Reserved Instance Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `veneer_reserved_instance_data_available` | Gauge | -- | `1` if Lumina is exposing RI metrics, `0` if not. |
| `veneer_reserved_instance_count` | Gauge | `instance_type`, `region` | Number of Reserved Instances detected by instance type and region. |

## Savings Plan Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `veneer_savings_plan_utilization_percent` | Gauge | `type`, `instance_family`, `region` | Savings Plan utilization percentage. |
| `veneer_savings_plan_remaining_capacity_dollars` | Gauge | `type`, `instance_family`, `region` | Savings Plan remaining capacity in dollars per hour. |

**Label values:**

| Label | Values | Description |
|-------|--------|-------------|
| `type` | SP type identifier | Type of Savings Plan |
| `instance_family` | Family name or `all` | Instance family (or `all` for Compute SPs) |
| `region` | AWS region or `global` | Region scope |

## NodeOverlay Lifecycle Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `veneer_overlay_operations_total` | Counter | `operation`, `capacity_type` | Total NodeOverlay operations. |
| `veneer_overlay_operation_errors_total` | Counter | `operation`, `error_type` | Total NodeOverlay operation errors. |
| `veneer_overlay_count` | Gauge | `capacity_type` | Current number of NodeOverlays managed by Veneer. |

**Label values:**

| Label | Values | Description |
|-------|--------|-------------|
| `operation` | `create`, `update`, `delete` | Type of overlay operation |
| `capacity_type` | `compute_savings_plan`, `ec2_instance_savings_plan`, `reserved_instance`, `preference` | Capacity type the overlay targets |
| `error_type` | `validation`, `api`, `not_found` | Type of error encountered |

## Prometheus Query Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `veneer_prometheus_query_duration_seconds` | Histogram | `query_type` | Duration of Prometheus queries to Lumina. Uses default Prometheus buckets. |
| `veneer_prometheus_query_errors_total` | Counter | `query_type` | Total Prometheus query errors. |
| `veneer_prometheus_query_result_count` | Gauge | `query_type` | Number of results returned by the last Prometheus query. |

**Label values for `query_type`:**

| Value | Description |
|-------|-------------|
| `sp_utilization` | Savings Plan utilization query |
| `sp_capacity` | Savings Plan remaining capacity query |
| `ri` | Reserved Instance count query |
| `data_freshness` | Lumina data freshness check |

## Configuration Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `veneer_config_overlays_disabled` | Gauge | -- | `1` if overlay creation is disabled (dry-run mode), `0` if enabled. |
| `veneer_config_utilization_threshold_percent` | Gauge | -- | Configured utilization threshold for overlay deletion. |

## Info Metric

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `veneer_info` | Gauge | `version`, `disabled_mode` | Controller information. Always set to `1`. |

## Example PromQL Queries

### Reconciliation Health

```promql
# Reconciliation error rate (last 5 minutes)
rate(veneer_reconciliation_total{result="error"}[5m])
/ rate(veneer_reconciliation_total[5m])

# Average reconciliation duration
rate(veneer_reconciliation_duration_seconds_sum[5m])
/ rate(veneer_reconciliation_duration_seconds_count[5m])
```

### Data Source Health

```promql
# Alert if Lumina data is unavailable
veneer_lumina_data_available == 0

# Data freshness in minutes
veneer_lumina_data_freshness_seconds / 60
```

### Overlay Activity

```promql
# Overlay creation rate by capacity type
rate(veneer_overlay_operations_total{operation="create"}[1h])

# Current overlay count
veneer_overlay_count

# Overlay operation error rate
rate(veneer_overlay_operation_errors_total[5m])
```

### Savings Plans Monitoring

```promql
# SP utilization across all types
veneer_savings_plan_utilization_percent

# Remaining SP capacity ($/hour)
veneer_savings_plan_remaining_capacity_dollars
```

## Grafana Dashboard

You can build a Grafana dashboard using these metrics. Key panels to include:

1. **Reconciliation Status** -- Success/error rate over time
2. **Lumina Data Freshness** -- Gauge showing data age
3. **Overlay Count** -- Breakdown by capacity type
4. **Decision Activity** -- Create vs delete decisions over time
5. **Prometheus Query Performance** -- Query latency and error rates
6. **SP Utilization** -- Per-type utilization percentages
