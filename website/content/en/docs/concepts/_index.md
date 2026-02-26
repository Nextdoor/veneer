---
title: "Concepts"
description: "Core concepts behind Veneer's cost-aware provisioning approach."
weight: 20
---

This section explains Veneer's architecture and the technical details of how it influences Karpenter's instance selection.

- [Architecture]({{< relref "architecture" >}}) -- Data flow, reconciliation loops, and overlay lifecycle
- [Instance Selection Deep Dive]({{< relref "instance-selection" >}}) -- How Karpenter selects instances via the AWS CreateFleet API
- [Bin-Packing and NodeOverlay]({{< relref "binpacking" >}}) -- How bin-packing can filter instance types before NodeOverlay applies
- [Instance Preferences]({{< relref "preferences" >}}) -- Annotation-based instance type preferences on NodePools
