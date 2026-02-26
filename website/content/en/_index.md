---
title: Veneer
---

{{< blocks/cover title="Veneer" image_anchor="top" height="med" color="primary" >}}
<p class="lead mt-4">Cost-Aware Karpenter Provisioning</p>
<a class="btn btn-lg btn-secondary me-3 mb-4" href="https://github.com/nextdoor/veneer">
View on GitHub <i class="fab fa-github ms-2"></i>
</a>
{{< /blocks/cover >}}

{{% blocks/lead color="dark" %}}
Veneer is a Kubernetes controller that optimizes **Karpenter provisioning decisions** using real-time cost data.
It manages NodeOverlay resources based on AWS Reserved Instance and Savings Plans data from [Lumina](https://oss.nextdoor.com/lumina) — steering Karpenter toward the most cost-effective instance types.
{{% /blocks/lead %}}

{{% blocks/section color="white" type="row" %}}

{{% blocks/feature icon="fa-solid fa-dollar-sign" title="Cost-Aware Scheduling" %}}
Automatically creates and manages Karpenter NodeOverlay resources to prioritize instance types covered by Reserved Instances and Savings Plans.
{{% /blocks/feature %}}

{{% blocks/feature icon="fa-solid fa-arrows-rotate" title="Real-Time Optimization" %}}
Continuously reconciles with Lumina's cost metrics to keep provisioning decisions aligned with your current RI/SP capacity and utilization.
{{% /blocks/feature %}}

{{% blocks/feature icon="fa-solid fa-plug" title="Seamless Karpenter Integration" %}}
Works alongside Karpenter's existing bin-packing and scheduling logic. Veneer influences instance selection without replacing Karpenter's core functionality.
{{% /blocks/feature %}}

{{% /blocks/section %}}

{{% blocks/section color="light" %}}
## Quick Start

```bash
helm repo add veneer https://oss.nextdoor.com/veneer
helm repo update
helm install veneer veneer/veneer
```
{{% /blocks/section %}}
