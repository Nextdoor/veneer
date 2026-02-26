---
title: "Getting Started"
description: "Install and configure Veneer for cost-aware Karpenter provisioning."
weight: 10
---

Before installing Veneer, ensure the following prerequisites are in place:

- **Karpenter** (v0.32+) -- Veneer manages NodeOverlay resources that Karpenter consumes for instance selection decisions.
- **Lumina** -- Provides the AWS Savings Plan and Reserved Instance cost metrics that Veneer queries via Prometheus. See the [Lumina documentation](https://github.com/Nextdoor/lumina) for setup instructions.
- **Prometheus** -- Lumina exposes its metrics to Prometheus, and Veneer queries Prometheus to retrieve them. Any Prometheus-compatible server works.

The installation guide walks you through deploying Veneer via Helm, configuring it to connect to your Prometheus instance, and verifying that NodeOverlays are being created correctly.
