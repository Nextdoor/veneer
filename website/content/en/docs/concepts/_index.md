---
title: "Concepts"
description: "Core concepts behind Veneer's cost-aware provisioning approach."
weight: 20
---

Veneer bridges the gap between Lumina's real-time cost data and Karpenter's provisioning decisions. It watches Prometheus for Savings Plan and Reserved Instance utilization metrics, then creates and manages **NodeOverlay** custom resources that adjust Karpenter's effective instance pricing. This steers Karpenter toward cost-optimal instance types without replacing its core scheduling and bin-packing logic.

The key mechanism is the NodeOverlay CRD: each overlay targets a set of instance types via label requirements and applies a price adjustment. Karpenter uses these adjusted prices as Priority values in the AWS CreateFleet API, causing AWS to prefer the instances Veneer has identified as cost-effective.

The pages in this section cover:

- **[Architecture]({{< relref "architecture" >}})** -- The end-to-end data flow from Lumina through Veneer to Karpenter, the two reconciliation loops (Metrics Reconciler and NodePool Reconciler), and the overlay lifecycle.
- **[Instance Selection Deep Dive]({{< relref "instance-selection" >}})** -- How Karpenter translates NodeOverlay price adjustments into AWS CreateFleet Priority values, and how allocation strategies affect the final instance choice.
- **[Bin-Packing and NodeOverlay]({{< relref "binpacking" >}})** -- How Karpenter's bin-packing step can filter out instance types before NodeOverlay price adjustments take effect, and what that means for provisioning outcomes.
- **[Instance Preferences]({{< relref "preferences" >}})** -- How to use `veneer.io/preference.N` annotations on NodePools to express instance type preferences independent of cost data.
