---
title: "Reference"
description: "Configuration reference, metrics catalog, Helm chart values, and CRD specification."
weight: 30
---

This section provides detailed reference documentation for every configurable aspect of Veneer, the metrics it exposes, and the custom resources it manages.

- **[Configuration]({{< relref "configuration" >}})** -- All configuration options (YAML keys, environment variables, CLI flags) with defaults and validation rules. Start here if you need to tune Veneer's behavior.
- **[Metrics]({{< relref "metrics" >}})** -- Complete catalog of Prometheus metrics exposed by Veneer, including reconciliation health, decision tracking, overlay lifecycle, and example PromQL queries.
- **[Helm Chart]({{< relref "helm-chart" >}})** -- Full Helm values reference for deploying Veneer, including security context defaults, resource recommendations, and example production/development configurations.
- **[NodeOverlay CRD]({{< relref "nodeoverlay" >}})** -- The NodeOverlay custom resource specification: fields, weight system, naming conventions, and example manifests for each overlay type.
