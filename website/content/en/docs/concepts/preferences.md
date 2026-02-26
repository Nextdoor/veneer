---
title: "Instance Preferences"
description: "Define instance type preferences on NodePools using annotations to influence Karpenter provisioning."
weight: 40
---

Instance preferences allow you to express instance type preferences directly on Karpenter NodePools using annotations. Veneer watches NodePools and generates [NodeOverlay]({{< relref "../reference/nodeoverlay" >}}) resources for each preference, influencing Karpenter's provisioning decisions.

{{% pageinfo %}}
**NodeOverlays are preferences, not rules.** When Veneer creates a NodeOverlay with a price adjustment, it influences but does not guarantee instance selection. See [Instance Selection Deep Dive]({{< relref "instance-selection" >}}) for how AWS makes the final decision.
{{% /pageinfo %}}

## Annotation Format

```
veneer.io/preference.N: "<matcher> [<matcher>...] adjust=[+-]N%"
```

Where:
- `N` is a positive integer (1-9 recommended) that determines overlay weight/priority
- `<matcher>` is `key=value1,value2` or `key!=value` or `key>value` or `key<value`
- `adjust` specifies the price adjustment percentage

## Example NodePool

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

## Generated NodeOverlay

For preference `veneer.io/preference.1` on the NodePool above, Veneer generates:

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

## Supported Labels

The following Karpenter and Kubernetes labels can be used in matchers:

| Label | Description | Example Values |
|-------|-------------|----------------|
| `karpenter.k8s.aws/instance-family` | Instance family | `c7a`, `m7g`, `r6i` |
| `karpenter.k8s.aws/instance-category` | Instance category | `c`, `m`, `r`, `t` |
| `karpenter.k8s.aws/instance-generation` | Instance generation number | `6`, `7`, `8` |
| `karpenter.k8s.aws/instance-size` | Instance size | `large`, `xlarge`, `2xlarge` |
| `karpenter.k8s.aws/instance-cpu` | Number of vCPUs | `4`, `8`, `16` |
| `karpenter.k8s.aws/instance-cpu-manufacturer` | CPU manufacturer | `intel`, `amd`, `aws` |
| `karpenter.k8s.aws/instance-memory` | Memory in MiB | `8192`, `16384` |
| `kubernetes.io/arch` | Architecture | `amd64`, `arm64` |
| `karpenter.sh/capacity-type` | Capacity type | `on-demand`, `spot` |
| `node.kubernetes.io/instance-type` | Specific instance type | `m5.xlarge`, `c7g.2xlarge` |

## Operators

| Syntax | Kubernetes Operator | Description | Example |
|--------|---------------------|-------------|---------|
| `=` | `In` | Match any of the values | `instance-family=c7a,c7g` |
| `!=` | `NotIn` | Exclude all of the values | `instance-family!=t3,t3a` |
| `>` | `Gt` | Greater than (numeric) | `instance-cpu>4` |
| `<` | `Lt` | Less than (numeric) | `instance-cpu<64` |

## Multiple Matchers

You can combine multiple matchers in a single preference to create compound requirements. All matchers must match for the overlay to apply:

```yaml
# Prefer m7g instances on ARM64 with >= 8 vCPUs
veneer.io/preference.1: "karpenter.k8s.aws/instance-family=m7g kubernetes.io/arch=arm64 karpenter.k8s.aws/instance-cpu>7 adjust=-40%"
```

This generates a NodeOverlay with three requirements (plus the NodePool selector), all of which must be satisfied for the price adjustment to apply.

## Weight and Priority

The number `N` in `veneer.io/preference.N` becomes the overlay weight:

- **Lower numbers = lower weight** (lower priority)
- **Higher numbers = higher weight** (higher priority)

### Weight Hierarchy

| Overlay Type | Default Weight | Priority |
|-------------|----------------|----------|
| Reserved Instance overlays | 30 | Highest (most specific) |
| EC2 Instance SP overlays | 20 | Medium |
| Compute SP overlays | 10 | Lower |
| Preference overlays | N (1-9) | Lowest |

{{% pageinfo color="warning" %}}
Keep preference numbers below 10 to ensure RI/SP overlays (backed by real AWS capacity data) take precedence over user-defined preferences.
{{% /pageinfo %}}

## Preference Lifecycle

| Event | Action |
|-------|--------|
| Preference annotation added to NodePool | Veneer creates a NodeOverlay |
| Preference annotation value changed | Veneer updates the NodeOverlay |
| Preference annotation removed | Veneer deletes the NodeOverlay |
| NodePool deleted | NodeOverlay is garbage collected via owner reference |

## Common Patterns

### Prefer ARM64 (Graviton)

```yaml
annotations:
  veneer.io/preference.1: "kubernetes.io/arch=arm64 adjust=-30%"
```

### Prefer Specific Instance Families

```yaml
annotations:
  veneer.io/preference.1: "karpenter.k8s.aws/instance-family=c7g,m7g adjust=-25%"
```

### Prefer Latest Generation

```yaml
annotations:
  veneer.io/preference.1: "karpenter.k8s.aws/instance-generation>6 adjust=-15%"
```

### Avoid Small Instances

```yaml
annotations:
  veneer.io/preference.1: "karpenter.k8s.aws/instance-cpu>7 adjust=-10%"
```

### Layered Preferences

You can stack preferences with increasing specificity and discounts:

```yaml
annotations:
  # Slight preference for ARM64
  veneer.io/preference.1: "kubernetes.io/arch=arm64 adjust=-10%"
  # Stronger preference for Graviton 7th gen
  veneer.io/preference.2: "karpenter.k8s.aws/instance-generation=7 kubernetes.io/arch=arm64 adjust=-20%"
  # Strongest preference for c7g family specifically
  veneer.io/preference.3: "karpenter.k8s.aws/instance-family=c7g adjust=-30%"
```

## Disabling Preferences

Preference processing can be disabled globally via configuration:

```yaml
# config.yaml
preferences:
  enabled: false
```

When disabled, the NodePool reconciler will not generate overlays from `veneer.io/preference.N` annotations. Existing preference overlays will be cleaned up.
