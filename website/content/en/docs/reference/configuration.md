---
title: "Configuration"
description: "All Veneer configuration options, environment variables, CLI flags, and defaults."
weight: 10
---

Veneer is configured via YAML file, environment variables, or CLI flags. For Helm-based deployments, configuration values are passed through the `config` section of the [Helm chart values]({{< relref "helm-chart" >}}). Configuration precedence (highest to lowest):

1. CLI flags
2. Environment variables (`VENEER_*` prefix)
3. Configuration file values
4. Default values

## Configuration File

Create a `config.yaml` (or use `config.example.yaml` as a starting point):

```yaml
# Prometheus URL for querying Lumina metrics
prometheusUrl: "http://prometheus:9090"

# Log level: debug, info, warn, or error
logLevel: "info"

# Metrics endpoint bind address
metricsBindAddress: ":8080"

# Health probe endpoint bind address
healthProbeBindAddress: ":8081"

# AWS configuration (REQUIRED)
aws:
  accountId: "123456789012"
  region: "us-west-2"

# Overlay management configuration
overlays:
  # Disabled mode: overlays created but won't match nodes
  disabled: false

  # Utilization threshold for overlay deletion (0-100)
  utilizationThreshold: 95.0

  # Overlay priority weights
  weights:
    reservedInstance: 30
    ec2InstanceSavingsPlan: 20
    computeSavingsPlan: 10

  # Overlay naming prefixes
  naming:
    reservedInstancePrefix: "cost-aware-ri"
    ec2InstanceSavingsPlanPrefix: "cost-aware-ec2-sp"
    computeSavingsPlanPrefix: "cost-aware-compute-sp"

# Instance preference configuration
preferences:
  enabled: true
```

## All Configuration Options

### Core Settings

| Option | YAML Key | Env Variable | Default | Description |
|--------|----------|-------------|---------|-------------|
| Prometheus URL | `prometheusUrl` | `VENEER_PROMETHEUS_URL` | `http://prometheus:9090` | URL of the Prometheus server for Lumina metrics |
| Log Level | `logLevel` | `VENEER_LOG_LEVEL` | `info` | Log verbosity: `debug`, `info`, `warn`, `error` |
| Metrics Bind Address | `metricsBindAddress` | `VENEER_METRICS_BIND_ADDRESS` | `:8080` | Address for the Prometheus metrics endpoint |
| Health Probe Bind Address | `healthProbeBindAddress` | `VENEER_HEALTH_PROBE_BIND_ADDRESS` | `:8081` | Address for health and readiness probes |

### AWS Settings (Required)

| Option | YAML Key | Env Variable | Default | Description |
|--------|----------|-------------|---------|-------------|
| Account ID | `aws.accountId` | `VENEER_AWS_ACCOUNT_ID` | (none) | 12-digit AWS account ID where this cluster runs |
| Region | `aws.region` | `VENEER_AWS_REGION` | (none) | AWS region where this cluster runs |

{{% pageinfo color="warning" %}}
Both `aws.accountId` and `aws.region` are **required**. Veneer uses them to scope Prometheus queries to only return RI/SP data from this specific account and region.
{{% /pageinfo %}}

### Overlay Management

| Option | YAML Key | Env Variable | Default | Description |
|--------|----------|-------------|---------|-------------|
| Disabled Mode | `overlays.disabled` | `VENEER_OVERLAY_DISABLED` | `false` | When `true`, overlays are created with an impossible requirement so they never match |
| Utilization Threshold | `overlays.utilizationThreshold` | -- | `95.0` | SP/RI utilization percentage at which overlays are deleted (0-100) |

### Overlay Weights

Weights control overlay precedence when multiple overlays target the same instances. Higher weight wins. See the [NodeOverlay CRD reference]({{< relref "nodeoverlay" >}}) for details on the weight system.

| Option | YAML Key | Default | Description |
|--------|----------|---------|-------------|
| Reserved Instance Weight | `overlays.weights.reservedInstance` | `30` | Weight for RI-backed overlays (instance-type specific) |
| EC2 Instance SP Weight | `overlays.weights.ec2InstanceSavingsPlan` | `20` | Weight for EC2 Instance SP overlays (family-specific) |
| Compute SP Weight | `overlays.weights.computeSavingsPlan` | `10` | Weight for Compute SP overlays (global) |

### Overlay Naming

| Option | YAML Key | Default | Description |
|--------|----------|---------|-------------|
| RI Prefix | `overlays.naming.reservedInstancePrefix` | `cost-aware-ri` | Name prefix for RI overlays (e.g., `cost-aware-ri-m5-xlarge-us-west-2`) |
| EC2 Instance SP Prefix | `overlays.naming.ec2InstanceSavingsPlanPrefix` | `cost-aware-ec2-sp` | Name prefix for EC2 Instance SP overlays |
| Compute SP Prefix | `overlays.naming.computeSavingsPlanPrefix` | `cost-aware-compute-sp` | Name prefix for Compute SP overlays |

### Instance Preferences

| Option | YAML Key | Default | Description |
|--------|----------|---------|-------------|
| Enabled | `preferences.enabled` | `true` | Whether to process `veneer.io/preference.N` annotations on NodePools |

## Environment Variables

All core settings can be overridden via environment variables:

```bash
export VENEER_PROMETHEUS_URL="http://prometheus.example.com:9090"
export VENEER_LOG_LEVEL="debug"
export VENEER_METRICS_BIND_ADDRESS=":9090"
export VENEER_HEALTH_PROBE_BIND_ADDRESS=":9091"
export VENEER_AWS_ACCOUNT_ID="123456789012"
export VENEER_AWS_REGION="us-west-2"
export VENEER_OVERLAY_DISABLED="true"
```

## CLI Flags

Command-line flags override both config file and environment variables:

```bash
./bin/manager --config=config.yaml --overlay-disabled
./bin/manager --help  # View all available flags
```

## Local Development Configuration

For local development with `kubectl port-forward`:

```yaml
# config.local.yaml
prometheusUrl: "http://localhost:9090"
logLevel: "debug"
metricsBindAddress: ":8080"
healthProbeBindAddress: ":8081"
aws:
  accountId: "123456789012"
  region: "us-west-2"
overlays:
  disabled: false
```

```bash
# Port-forward to Prometheus
kubectl port-forward -n lumina-system svc/lumina-prometheus 9090:9090

# Run with local config
make run
```

## Validation

Veneer validates configuration at startup:

- `prometheusUrl` must be non-empty
- `aws.accountId` must be exactly 12 digits
- `aws.region` must be non-empty
- `logLevel` must be one of: `debug`, `info`, `warn`, `error`
- `overlays.utilizationThreshold` must be between 0 and 100
- All overlay weights must be non-negative
