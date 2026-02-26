---
title: "NodeOverlay CRD"
description: "NodeOverlay custom resource specification, weight system, and naming conventions."
weight: 40
---

NodeOverlay is a Karpenter custom resource (`karpenter.sh/v1alpha1`) that allows adjusting instance type pricing used during provisioning. Veneer creates and manages NodeOverlay resources to influence which instances Karpenter selects.

## API Definition

```yaml
apiVersion: karpenter.sh/v1alpha1
kind: NodeOverlay
metadata:
  name: <overlay-name>
  labels:
    app.kubernetes.io/managed-by: veneer
    veneer.io/type: <cost-aware|preference>
    veneer.io/source-nodepool: <nodepool-name>  # For preference overlays
spec:
  # Requirements that an instance must match for this overlay to apply
  requirements:
    - key: <label-key>
      operator: <In|NotIn|Gt|Lt|Exists|DoesNotExist>
      values: [<value1>, <value2>, ...]

  # Price adjustment (percentage string)
  priceAdjustment: "<adjustment>"

  # Weight for overlay precedence (higher = higher priority)
  weight: <integer>
```

## Spec Fields

### `spec.requirements`

A list of node selector requirements that an instance type must match for this overlay's price adjustment to apply. Uses the same requirement format as Karpenter NodePool requirements.

| Field | Type | Description |
|-------|------|-------------|
| `key` | string | Label key to match (e.g., `karpenter.k8s.aws/instance-family`) |
| `operator` | string | Comparison operator: `In`, `NotIn`, `Gt`, `Lt`, `Exists`, `DoesNotExist` |
| `values` | []string | Values to match against |

All requirements must be satisfied for the overlay to apply to an instance type.

### `spec.priceAdjustment`

A string representing the price adjustment to apply. Veneer uses percentage adjustments:

- `"-30%"` -- Reduce the effective price by 30% (makes instance more preferred)
- `"+20%"` -- Increase the effective price by 20% (makes instance less preferred)

The adjusted price becomes the Priority value in the AWS CreateFleet API call. Lower Priority = higher preference.

### `spec.weight`

An integer that determines overlay precedence. When multiple overlays match the same instance type, the overlay with the **highest weight** takes effect.

## Weight System

Veneer uses a tiered weight system to ensure the most specific capacity data takes precedence:

| Overlay Type | Default Weight | Scope | Naming Prefix |
|-------------|----------------|-------|---------------|
| Reserved Instance | 30 | Instance-type + region specific | `cost-aware-ri-` |
| EC2 Instance Savings Plan | 20 | Instance-family + region specific | `cost-aware-ec2-sp-` |
| Compute Savings Plan | 10 | Global (all families, all regions) | `cost-aware-compute-sp-` |
| Preference | 1-9 | User-defined | `pref-` |

This hierarchy ensures that when both an RI and a Compute SP apply to the same instance type, the more specific RI overlay (weight 30) takes precedence over the general Compute SP overlay (weight 10).

## Naming Conventions

### Cost-Aware Overlays

Veneer generates overlay names from the capacity type prefix and identifying attributes:

| Type | Pattern | Example |
|------|---------|---------|
| Reserved Instance | `{prefix}-{instance-type}-{region}` | `cost-aware-ri-m5-xlarge-us-west-2` |
| EC2 Instance SP | `{prefix}-{family}-{region}` | `cost-aware-ec2-sp-m5-us-west-2` |
| Compute SP | `{prefix}-global` | `cost-aware-compute-sp-global` |

Naming prefixes are configurable via the `overlays.naming` configuration. See [Configuration Reference]({{< relref "configuration" >}}).

### Preference Overlays

Preference overlays use the NodePool name and preference number:

| Pattern | Example |
|---------|---------|
| `pref-{nodepool-name}-{N}` | `pref-my-workload-1` |

## Labels

All Veneer-managed overlays carry these labels:

| Label | Value | Description |
|-------|-------|-------------|
| `app.kubernetes.io/managed-by` | `veneer` | Identifies Veneer-managed overlays |
| `veneer.io/type` | `cost-aware` or `preference` | Overlay source type |
| `veneer.io/source-nodepool` | NodePool name | (Preference overlays only) Source NodePool |

These labels are used for:
- Listing all Veneer-managed overlays: `kubectl get nodeoverlays -l app.kubernetes.io/managed-by=veneer`
- Filtering by type: `kubectl get nodeoverlays -l veneer.io/type=preference`
- Finding overlays for a NodePool: `kubectl get nodeoverlays -l veneer.io/source-nodepool=my-workload`

## Examples

### Cost-Aware: Reserved Instance Overlay

Created when Lumina detects active Reserved Instances for `m5.xlarge` in `us-west-2`:

```yaml
apiVersion: karpenter.sh/v1alpha1
kind: NodeOverlay
metadata:
  name: cost-aware-ri-m5-xlarge-us-west-2
  labels:
    app.kubernetes.io/managed-by: veneer
    veneer.io/type: cost-aware
spec:
  requirements:
    - key: node.kubernetes.io/instance-type
      operator: In
      values: ["m5.xlarge"]
    - key: topology.kubernetes.io/region
      operator: In
      values: ["us-west-2"]
    - key: karpenter.sh/capacity-type
      operator: In
      values: ["on-demand"]
  priceAdjustment: "-100%"
  weight: 30
```

### Cost-Aware: EC2 Instance Savings Plan Overlay

Created when Lumina detects an EC2 Instance Savings Plan covering the `m5` family in `us-west-2` with remaining capacity:

```yaml
apiVersion: karpenter.sh/v1alpha1
kind: NodeOverlay
metadata:
  name: cost-aware-ec2-sp-m5-us-west-2
  labels:
    app.kubernetes.io/managed-by: veneer
    veneer.io/type: cost-aware
spec:
  requirements:
    - key: karpenter.k8s.aws/instance-family
      operator: In
      values: ["m5"]
    - key: topology.kubernetes.io/region
      operator: In
      values: ["us-west-2"]
    - key: karpenter.sh/capacity-type
      operator: In
      values: ["on-demand"]
  priceAdjustment: "-30%"
  weight: 20
```

### Cost-Aware: Compute Savings Plan Overlay

Created when Lumina detects a Compute Savings Plan with remaining capacity:

```yaml
apiVersion: karpenter.sh/v1alpha1
kind: NodeOverlay
metadata:
  name: cost-aware-compute-sp-global
  labels:
    app.kubernetes.io/managed-by: veneer
    veneer.io/type: cost-aware
spec:
  requirements:
    - key: karpenter.sh/capacity-type
      operator: In
      values: ["on-demand"]
  priceAdjustment: "-15%"
  weight: 10
```

### Preference Overlay

Created from a `veneer.io/preference.2` annotation on a NodePool:

```yaml
apiVersion: karpenter.sh/v1alpha1
kind: NodeOverlay
metadata:
  name: pref-my-workload-2
  labels:
    app.kubernetes.io/managed-by: veneer
    veneer.io/type: preference
    veneer.io/source-nodepool: my-workload
spec:
  requirements:
    - key: karpenter.sh/nodepool
      operator: In
      values: ["my-workload"]
    - key: kubernetes.io/arch
      operator: In
      values: ["arm64"]
  priceAdjustment: "-30%"
  weight: 2
```

### Disabled Mode Overlay

When `overlays.disabled: true`, overlays include an impossible requirement:

```yaml
apiVersion: karpenter.sh/v1alpha1
kind: NodeOverlay
metadata:
  name: cost-aware-ri-m5-xlarge-us-west-2
spec:
  requirements:
    - key: node.kubernetes.io/instance-type
      operator: In
      values: ["m5.xlarge"]
    - key: veneer.io/disabled
      operator: In
      values: ["true"]  # No node will ever have this label
  priceAdjustment: "-100%"
  weight: 30
```

## How NodeOverlay Affects Karpenter

When a NodeOverlay exists, Karpenter changes its behavior in two ways:

1. **Price Adjustment**: The overlay's `priceAdjustment` modifies the effective price of matching instance types. This adjusted price is used for sorting and as the Priority value in the CreateFleet API call.

2. **Allocation Strategy Change**: The presence of any NodeOverlay switches the allocation strategy:
   - Spot: `price-capacity-optimized` becomes `capacity-optimized-prioritized`
   - On-Demand: `lowest-price` becomes `prioritized`

   This ensures AWS uses the Priority values (adjusted prices) when selecting instances.

See [Instance Selection Deep Dive]({{< relref "../concepts/instance-selection" >}}) for the full technical explanation.
