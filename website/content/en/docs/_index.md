---
title: "Documentation"
description: "Comprehensive documentation for the Veneer cost-aware Karpenter provisioning controller."
weight: 20
---

Veneer is a Kubernetes controller that optimizes [Karpenter](https://karpenter.sh/) provisioning decisions by managing NodeOverlay resources based on real-time AWS Reserved Instance and Savings Plans data from [Lumina](https://github.com/Nextdoor/lumina).

### [Getting Started]({{< relref "getting-started" >}})

Install Veneer and start optimizing your Karpenter provisioning costs.

### [Concepts]({{< relref "concepts" >}})

Understand Veneer's architecture, instance selection flow, and bin-packing behavior.
[Architecture]({{< relref "concepts/architecture" >}}) | [Instance Selection]({{< relref "concepts/instance-selection" >}}) | [Bin-Packing]({{< relref "concepts/binpacking" >}}) | [Preferences]({{< relref "concepts/preferences" >}})

### [Reference]({{< relref "reference" >}})

Configuration options, Helm chart values, Prometheus metrics, and the NodeOverlay CRD.
[Configuration]({{< relref "reference/configuration" >}}) | [Helm Chart]({{< relref "reference/helm-chart" >}}) | [Metrics]({{< relref "reference/metrics" >}}) | [NodeOverlay CRD]({{< relref "reference/nodeoverlay" >}})

### [Troubleshooting]({{< relref "troubleshooting" >}})

Debug common issues and diagnose data freshness problems.

### [Development]({{< relref "development" >}})

Local setup, testing, and contributing to Veneer.

## How It Works

```
Lumina --> Exposes SP/RI metrics to Prometheus
             |
Veneer --> Queries metrics, manages NodeOverlays
             |
Karpenter --> Uses adjusted pricing for provisioning decisions
```

Veneer watches Lumina metrics and creates/updates/deletes Karpenter NodeOverlay CRs to:

- **Prefer RI/SP-covered on-demand instances** when cost-effective
- **Fall back to spot** when RI/SP capacity is exhausted
- **Avoid provisioning thrashing** with smart debouncing
- **Express instance preferences** via NodePool annotations
