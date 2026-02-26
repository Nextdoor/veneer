---
title: "Troubleshooting"
description: "Common issues, debugging steps, and data freshness for Veneer."
weight: 40
---

## Common Issues

### No NodeOverlays Created

**Symptom:** Veneer is running but no NodeOverlays appear.

**Check data availability:**
```bash
# Check if Lumina data is available
kubectl port-forward -n veneer-system svc/veneer-metrics 8080:8080
curl -s http://localhost:8080/metrics | grep veneer_lumina_data_available
# Expected: veneer_lumina_data_available 1
```

If `veneer_lumina_data_available` is `0`:
1. Verify Lumina is running: `kubectl get pods -n lumina-system`
2. Verify Prometheus is scraping Lumina: check Prometheus targets UI
3. Verify the Prometheus URL is correct in Veneer's [configuration]({{< relref "reference/configuration" >}})

**Check utilization threshold:**
```bash
curl -s http://localhost:8080/metrics | grep veneer_savings_plan_utilization
```

If utilization is above the configured [utilization threshold]({{< relref "reference/configuration" >}}) (default 95%), overlays will not be created because the pre-paid capacity is fully consumed.

**Check disabled mode:**
```bash
curl -s http://localhost:8080/metrics | grep veneer_config_overlays_disabled
# Expected: veneer_config_overlays_disabled 0
```

If `1`, Veneer is in disabled mode. Overlays are created but with an impossible requirement so they never match. See the [NodeOverlay CRD reference]({{< relref "reference/nodeoverlay" >}}) for details on disabled mode overlays.

### x86 Selected Despite ARM64 Preference

**Symptom:** You configured a preference for ARM64 but Karpenter still provisions x86 instances.

This can happen for several reasons:

1. **Bin-packing filtered out ARM64** -- If the aggregate CPU requirement falls in the 97-128 vCPU range, only x86 32xlarge instances are available (Graviton has no 32xlarge). See [Bin-Packing and NodeOverlay]({{< relref "concepts/binpacking" >}}).

2. **ARM64 spot capacity exhausted** -- Even with lower Priority values, AWS will select x86 if ARM64 spot pools lack capacity. Check CloudTrail for `InsufficientInstanceCapacity` errors.

3. **NodeOverlay not applied** -- Verify the overlay exists and matches the instance types:
   ```bash
   kubectl get nodeoverlays -l veneer.io/type=preference
   ```
   See the [NodeOverlay CRD reference]({{< relref "reference/nodeoverlay" >}}) for label and requirement details.

See the [Bin-Packing]({{< relref "concepts/binpacking" >}}) page for diagnostic steps and solutions.

### "Failed to Query Data Freshness" Errors

**Symptom:** Log errors about Prometheus connectivity.

```bash
# Check Prometheus connectivity
curl http://<prometheus-url>:9090/-/healthy

# Check Prometheus targets
curl -s http://<prometheus-url>:9090/api/v1/targets | jq '.data.activeTargets[] | {job: .labels.job, health: .health}'

# Verify Lumina metrics exist
curl -s 'http://<prometheus-url>:9090/api/v1/query?query=savings_plan_remaining_capacity' | jq '.data.result | length'
```

If running locally with port-forward:
```bash
# Verify port-forward is active
lsof -i:9090

# Re-establish if needed
kubectl port-forward -n lumina-system svc/lumina-prometheus 9090:9090
```

### "Context Deadline Exceeded" Errors

**Symptom:** Timeout errors when querying Prometheus.

1. Check that Lumina is running and healthy:
   ```bash
   kubectl get pods -n lumina-system
   ```
2. Verify Prometheus has scraped recent metrics
3. Check Prometheus query performance -- some queries may be slow on large datasets

### Port Already in Use

**Symptom:** Veneer fails to start with a bind error.

```bash
# Find process using the port
lsof -ti:8081 | xargs kill -9

# Or change the port in config (see Configuration Reference)
healthProbeBindAddress: ":8082"
```

Port values are configurable via the [Configuration reference]({{< relref "reference/configuration" >}}) or [Helm chart values]({{< relref "reference/helm-chart" >}}).

### Overlays Created But Karpenter Ignores Them

**Symptom:** NodeOverlays exist but provisioning behavior doesn't change.

1. **Verify Karpenter supports NodeOverlay**:
   ```bash
   kubectl get crd nodeoverlays.karpenter.sh
   ```

2. **Check overlay requirements match instance types**: The requirements in the overlay must match instances that Karpenter is considering. Verify with:
   ```bash
   kubectl get nodeoverlay <name> -o yaml
   ```

3. **Check allocation strategy in CloudTrail**: Look for `capacity-optimized-prioritized` (spot) or `prioritized` (on-demand) in CreateFleet requests. If you see `price-capacity-optimized` or `lowest-price`, NodeOverlay is not being applied.

### "No Matching Capacity" Warnings

**Symptom:** Veneer can't match Savings Plans utilization with capacity data.

1. Check that Lumina is exposing both utilization and capacity metrics
2. Verify ARNs match between metrics
3. Enable debug logging to see actual query results:
   ```yaml
   logLevel: "debug"
   ```

## Data Freshness

Veneer checks Lumina data freshness before each reconciliation. If data is stale (older than expected), Veneer skips the reconciliation cycle to avoid making decisions based on outdated information.

**Monitor freshness**:
```bash
curl -s http://localhost:8080/metrics | grep veneer_lumina_data_freshness_seconds
```

The `veneer_lumina_data_available` metric reports whether data is fresh enough to act on. See the [Metrics reference]({{< relref "reference/metrics" >}}) for the full list of available metrics.

**Common causes of stale data**:
- Lumina controller is not running or is unhealthy
- Prometheus is not scraping Lumina
- Network connectivity issues between Veneer and Prometheus

## Debugging with Logs

### Enable Debug Logging

```yaml
# config.yaml
logLevel: "debug"
```

Or via environment variable:
```bash
export VENEER_LOG_LEVEL=debug
```

### Key Log Messages

| Log Message | Meaning |
|-------------|---------|
| `Starting metrics reconciler` | Controller started successfully |
| `Reconciliation complete` | A reconciliation cycle finished |
| `Lumina data is stale` | Data freshness check failed, skipping cycle |
| `Creating NodeOverlay` | An overlay is being created |
| `Deleting NodeOverlay` | An overlay is being removed |
| `SP utilization at/above threshold` | SP is fully utilized, no overlay needed |
| `SP utilization below threshold` | SP has remaining capacity, overlay created |

### Useful kubectl Commands

```bash
# View Veneer logs
kubectl logs -n veneer-system -l app.kubernetes.io/name=veneer --tail=100

# Follow logs in real-time
kubectl logs -n veneer-system -l app.kubernetes.io/name=veneer -f

# List all Veneer-managed overlays
kubectl get nodeoverlays -l app.kubernetes.io/managed-by=veneer

# Describe a specific overlay
kubectl describe nodeoverlay cost-aware-ec2-sp-m5-us-west-2

# Check Veneer metrics
kubectl port-forward -n veneer-system svc/veneer-metrics 8080:8080
curl -s http://localhost:8080/metrics | grep veneer_
```
