# Veneer

> Cost-aware Karpenter provisioning via NodeOverlay management

Veneer is a Kubernetes controller that optimizes Karpenter provisioning decisions by managing NodeOverlay resources based on real-time AWS Reserved Instance and Savings Plans data from [Lumina](https://github.com/Nextdoor/lumina).

## Status

**Phase 1: In Development** - Controller skeleton and Prometheus integration in progress.

See [RFC-0003](https://github.com/Nextdoor/cloudeng/blob/main/rfcs/RFC-0003-karpenter-cost-aware-provisioning.md) for full design.

## Overview

Veneer watches Lumina metrics and creates/updates/deletes Karpenter NodeOverlay CRs to:
- Prefer RI/SP-covered on-demand instances when cost-effective
- Fall back to spot when RI/SP capacity exhausted
- Avoid provisioning thrashing with smart debouncing

## Architecture

```
Lumina (RFC-0002) → Exposes SP/RI metrics to Prometheus
      ↓
Veneer (RFC-0003) → Queries metrics, manages NodeOverlays
      ↓
Karpenter → Uses adjusted pricing for provisioning decisions
```

## Understanding NodeOverlays

**NodeOverlays are preferences, not rules.** When Veneer creates a NodeOverlay with a price adjustment (e.g., `-30%` for ARM64), it influences but does not guarantee instance selection. Karpenter passes the adjusted prices as Priority values to the AWS EC2 Fleet API, which makes the final instance selection. AWS first ensures spot capacity is available, then selects among available pools based on Priority. This means if your preferred instance type (e.g., ARM64) has no spot capacity in any availability zone, AWS will select an alternative even if it has a worse (higher) Priority value. For a detailed technical explanation of this flow, see [Karpenter Instance Selection Deep Dive](docs/karpenter-instance-selection-deep-dive.md).

## Features

### Cost-Aware Provisioning (Lumina Integration)

Veneer queries Lumina metrics to automatically create NodeOverlays that adjust instance pricing based on RI/SP coverage:
- **Reserved Instances**: Highest priority (weight 30)
- **EC2 Instance Savings Plans**: Medium priority (weight 20)
- **Compute Savings Plans**: Lower priority (weight 10)

### Instance Preferences

Define instance type preferences directly on NodePools using annotations. Veneer watches NodePools and generates NodeOverlay resources for each preference.

**Annotation Format:**
```
veneer.io/preference.N: "<matcher> [<matcher>...] adjust=[+-]N%"
```

Where:
- `N` is a positive integer (1-9 recommended) that determines overlay weight/priority
- `<matcher>` is `key=value1,value2` or `key!=value` or `key>value` or `key<value`
- `adjust` specifies the price adjustment percentage

**Example NodePool:**
```yaml
apiVersion: karpenter.sh/v1
kind: NodePool
metadata:
  name: my-workload
  annotations:
    # Prefer c7a/c7g families with 20% discount
    veneer.io/preference.1: "karpenter.k8s.aws/instance-family=c7a,c7g adjust=-20%"
    # Prefer ARM64 architecture with 30% discount
    veneer.io/preference.2: "kubernetes.io/arch=arm64 adjust=-30%"
    # Combined matcher: m7g on ARM64 with 40% discount
    veneer.io/preference.3: "karpenter.k8s.aws/instance-family=m7g kubernetes.io/arch=arm64 adjust=-40%"
```

**Generated NodeOverlay:**
```yaml
apiVersion: karpenter.sh/v1alpha1
kind: NodeOverlay
metadata:
  name: pref-my-workload-1
  labels:
    app.kubernetes.io/managed-by: veneer
    veneer.io/type: preference
    veneer.io/source-nodepool: my-workload
spec:
  requirements:
    - key: karpenter.sh/nodepool
      operator: In
      values: ["my-workload"]
    - key: karpenter.k8s.aws/instance-family
      operator: In
      values: ["c7a", "c7g"]
  priceAdjustment: "-20%"
  weight: 1
```

**Supported Labels:**
| Label | Description |
|-------|-------------|
| `karpenter.k8s.aws/instance-family` | Instance family (c7a, m7g, etc.) |
| `karpenter.k8s.aws/instance-category` | Instance category (c, m, r, etc.) |
| `karpenter.k8s.aws/instance-generation` | Instance generation number |
| `karpenter.k8s.aws/instance-size` | Instance size (large, xlarge, etc.) |
| `karpenter.k8s.aws/instance-cpu` | Number of vCPUs |
| `karpenter.k8s.aws/instance-cpu-manufacturer` | CPU manufacturer (intel, amd, aws) |
| `karpenter.k8s.aws/instance-memory` | Memory in MiB |
| `kubernetes.io/arch` | Architecture (amd64, arm64) |
| `karpenter.sh/capacity-type` | Capacity type (on-demand, spot) |
| `node.kubernetes.io/instance-type` | Specific instance type |

**Operators:**
| Syntax | Kubernetes Operator | Description |
|--------|---------------------|-------------|
| `=` | `In` | Match any of the values |
| `!=` | `NotIn` | Exclude all of the values |
| `>` | `Gt` | Greater than (numeric) |
| `<` | `Lt` | Less than (numeric) |

**Weight Hierarchy:**
- Reserved Instance overlays: weight 30 (highest priority)
- EC2 Instance SP overlays: weight 20
- Compute SP overlays: weight 10
- Preference overlays: weight N (from annotation number)

Keep preference numbers below 10 to ensure RI/SP overlays take precedence.

**Lifecycle:**
- Overlays are created when preferences are added to a NodePool
- Overlays are updated when preference values change
- Overlays are deleted when preferences are removed or the NodePool is deleted
- Owner references ensure garbage collection on controller uninstall

## Development

See [DEVELOPMENT.md](DEVELOPMENT.md) for comprehensive development guide including:
- Local setup and prerequisites
- Running Veneer locally with kubectl port-forward
- Testing strategies (unit, integration, E2E)
- Debugging tips and troubleshooting
- Code style guidelines and contribution workflow

### Quick Start

```bash
# Build (auto-installs Go into ./bin/go/)
make build

# Run tests
make test

# Lint
make lint
```

No Go installation required - `make` handles it automatically.

### Running Locally

To run Veneer locally against a Kubernetes cluster:

1. **Port-forward to Prometheus/Lumina:**
   ```bash
   kubectl port-forward -n lumina-system svc/lumina-prometheus 9090:9090
   ```

2. **Run the controller:**
   ```bash
   make run
   ```

See [DEVELOPMENT.md](DEVELOPMENT.md) for detailed instructions, troubleshooting, and advanced topics

## Configuration

See [config.example.yaml](config.example.yaml) for all configuration options.

Local development uses `config.local.yaml`:

```yaml
prometheusUrl: "http://localhost:9090"  # Via port-forward
logLevel: "debug"                        # Verbose logging for development
```

## Contributing

This project will be open-sourced. See [DEVELOPMENT.md](DEVELOPMENT.md) for the full development guide and [CLAUDE.md](CLAUDE.md) for code style guidelines.

**Key Requirements:**
- Comprehensive test coverage with focus on integration tests
- All code must be open-source ready (no internal references)
- Follow conventional commit format
- Run `make lint` and `make test` before committing

## License

Apache 2.0 (to be confirmed)

## Credits

Built by the Platform Engineering team as a companion to [Lumina](https://github.com/Nextdoor/lumina).
