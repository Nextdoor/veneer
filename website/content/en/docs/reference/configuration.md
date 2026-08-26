---
title: "Configuration"
description: "All Veneer configuration options, environment variables, CLI flags, and defaults."
weight: 10
---

Veneer is configured via YAML, environment variables, or CLI flags. Helm deployments pass controller configuration under the chart's `config` value. Precedence is CLI flags, environment variables, configuration file, then defaults.

## Configuration File

```yaml
prometheusUrl: "http://prometheus:9090"
logLevel: "info"
metricsBindAddress: ":8080"
healthProbeBindAddress: ":8081"

aws:
  accountId: "123456789012"
  region: "us-west-2"

overlays:
  disabled: false
  utilizationThreshold: 95.0

  reservedInstance:
    enabled: true
    priceAdjustment: "-90%"

  ec2InstanceSavingsPlan:
    enabled: true
    priceAdjustment: "-90%"

  computeSavingsPlan:
    enabled: true
    priceAdjustment: "-50%"
    nodePoolSelector:
      names: []
    minRemainingCapacityDollars: 50
    minBelowThresholdDuration: 15m

  weights:
    reservedInstance: 30
    ec2InstanceSavingsPlan: 20
    computeSavingsPlan: 10

  naming:
    reservedInstancePrefix: "cost-aware-ri"
    ec2InstanceSavingsPlanPrefix: "cost-aware-ec2-sp"
    computeSavingsPlanPrefix: "cost-aware-compute-sp"

preferences:
  enabled: true
```

## Core Settings

| YAML Key | Env Variable | Default | Description |
|----------|--------------|---------|-------------|
| `prometheusUrl` | `VENEER_PROMETHEUS_URL` | `http://prometheus:9090` | Prometheus server containing Lumina metrics |
| `logLevel` | `VENEER_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error` |
| `metricsBindAddress` | `VENEER_METRICS_BIND_ADDRESS` | `:8080` | Metrics endpoint address |
| `healthProbeBindAddress` | `VENEER_HEALTH_PROBE_BIND_ADDRESS` | `:8081` | Health and readiness endpoint address |
| `aws.accountId` | `VENEER_AWS_ACCOUNT_ID` | none | Required 12-digit AWS account ID |
| `aws.region` | `VENEER_AWS_REGION` | none | Required AWS region |

## Overlay Management

| YAML Key | Env Variable | Default | Description |
|----------|--------------|---------|-------------|
| `overlays.disabled` | `VENEER_OVERLAY_DISABLED` | `false` | Add an impossible requirement to every generated overlay |
| `overlays.utilizationThreshold` | -- | `95.0` | Delete SP overlays at this utilization percentage |
| `overlays.reservedInstance.enabled` | `VENEER_OVERLAY_RESERVED_INSTANCE_ENABLED` | `true` | Generate Reserved Instance overlays |
| `overlays.reservedInstance.priceAdjustment` | `VENEER_OVERLAY_RESERVED_INSTANCE_PRICE_ADJUSTMENT` | `-90%` | Relative discount for RI-matching offerings |
| `overlays.ec2InstanceSavingsPlan.enabled` | `VENEER_OVERLAY_EC2_INSTANCE_SAVINGS_PLAN_ENABLED` | `true` | Generate EC2 Instance SP overlays |
| `overlays.ec2InstanceSavingsPlan.priceAdjustment` | `VENEER_OVERLAY_EC2_INSTANCE_SAVINGS_PLAN_PRICE_ADJUSTMENT` | `-90%` | Relative discount for EC2 Instance SP offerings |
| `overlays.computeSavingsPlan.enabled` | `VENEER_OVERLAY_COMPUTE_SAVINGS_PLAN_ENABLED` | `true` | Generate Compute SP overlays |
| `overlays.computeSavingsPlan.priceAdjustment` | `VENEER_OVERLAY_COMPUTE_SAVINGS_PLAN_PRICE_ADJUSTMENT` | `-50%` | Relative discount for Compute SP offerings |
| `overlays.computeSavingsPlan.nodePoolSelector.names` | -- | `[]` | Explicit NodePool name allowlist; empty targets all NodePools |
| `overlays.computeSavingsPlan.minRemainingCapacityDollars` | `VENEER_OVERLAY_COMPUTE_SAVINGS_PLAN_MIN_REMAINING_CAPACITY_DOLLARS` | `50` | Minimum unused hourly Compute SP commitment before creation |
| `overlays.computeSavingsPlan.minBelowThresholdDuration` | `VENEER_OVERLAY_COMPUTE_SAVINGS_PLAN_MIN_BELOW_THRESHOLD_DURATION` | `15m` | Continuous eligibility required before creation |

All `priceAdjustment` values must be percentage discounts strictly between `-100%` and `0%`. Relative discounts preserve instance-type price ordering; neither an absolute price nor a zero/positive adjustment is accepted. RI and EC2 Instance SP overlays default to a deeper `-90%` discount so covered on-demand capacity can still outrank typical Spot prices while retaining distinct instance prices. Compute SP defaults to `-50%` so Spot can remain cheaper in mixed-capacity pools. The Compute SP capacity floor and duration must be nonnegative.

{{% pageinfo color="warning" %}}
Compute Savings Plans apply broadly. In clusters with NodePools that permit both Spot and On-Demand capacity, disable the Compute SP overlay or scope it to on-demand-only NodePools. An unscoped Compute SP discount can change mixed-pool purchasing behavior.
{{% /pageinfo %}}

`nodePoolSelector` supports **explicit NodePool names only**. It does not evaluate Kubernetes metadata label selectors. This deliberate trade-off keeps the generated NodeOverlay requirement deterministic and avoids a separate NodePool label-resolution lifecycle. `names` has no environment-variable override because it is a list rather than a scalar.

The floor and duration affect creation only. An existing Compute SP overlay is removed immediately when utilization reaches the configured threshold or remaining capacity no longer meets the safety conditions.

## Overlay Weights and Naming

| YAML Key | Default | Description |
|----------|---------|-------------|
| `overlays.weights.reservedInstance` | `30` | RI overlay precedence |
| `overlays.weights.ec2InstanceSavingsPlan` | `20` | EC2 Instance SP overlay precedence |
| `overlays.weights.computeSavingsPlan` | `10` | Compute SP overlay precedence |
| `overlays.naming.reservedInstancePrefix` | `cost-aware-ri` | RI overlay name prefix |
| `overlays.naming.ec2InstanceSavingsPlanPrefix` | `cost-aware-ec2-sp` | EC2 Instance SP overlay name prefix |
| `overlays.naming.computeSavingsPlanPrefix` | `cost-aware-compute-sp` | Compute SP overlay name prefix |
| `preferences.enabled` | `true` | Process `veneer.io/preference.N` NodePool annotations |

Higher overlay weights win when multiple overlays match an offering.

## Environment Variables

Scalar settings can be overridden explicitly:

```bash
export VENEER_PROMETHEUS_URL="http://prometheus.example.com:9090"
export VENEER_LOG_LEVEL="debug"
export VENEER_AWS_ACCOUNT_ID="123456789012"
export VENEER_AWS_REGION="us-west-2"
export VENEER_OVERLAY_COMPUTE_SAVINGS_PLAN_ENABLED="false"
export VENEER_OVERLAY_COMPUTE_SAVINGS_PLAN_PRICE_ADJUSTMENT="-50%"
export VENEER_OVERLAY_COMPUTE_SAVINGS_PLAN_MIN_REMAINING_CAPACITY_DOLLARS="50"
export VENEER_OVERLAY_COMPUTE_SAVINGS_PLAN_MIN_BELOW_THRESHOLD_DURATION="15m"
```

The NodePool name list is YAML-only.

## CLI Flags

```bash
./bin/manager --config=config.yaml --overlay-disabled
./bin/manager --help
```

Only global disabled mode has a dedicated overlay CLI flag. Use YAML or the explicit scalar environment variables for per-type settings.

## Local Development Configuration

For local development with a port-forwarded Prometheus endpoint:

```yaml
prometheusUrl: "http://localhost:9090"
logLevel: "debug"
aws:
  accountId: "123456789012"
  region: "us-west-2"
overlays:
  disabled: false
  computeSavingsPlan:
    enabled: false
```

```bash
kubectl port-forward -n lumina-system svc/lumina-prometheus 9090:9090
make run
```

## Validation

Veneer validates configuration at startup:

- `prometheusUrl` must be non-empty.
- `aws.accountId` must contain exactly 12 digits.
- `aws.region` must be non-empty.
- `logLevel` must be `debug`, `info`, `warn`, or `error`.
- `overlays.utilizationThreshold` must be between 0 and 100.
- Per-type price adjustments must be discounts strictly between `-100%` and `0%`.
- Compute SP capacity floor and duration must be nonnegative.
- Overlay weights must be nonnegative.
